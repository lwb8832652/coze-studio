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

/* eslint-disable max-lines -- The reviewed canonical surface shares one transport owner. */

import { fetchStream } from '@coze-arch/fetch-stream';

import type {
  AppendWorkbenchMessageRequest,
  CancelWorkbenchRunRequest,
  ClearWorkbenchMemoriesRequest,
  CreateWorkbenchRunRequest,
  CreateWorkbenchThreadRequest,
  DeleteWorkbenchUploadRequest,
  ExportWorkbenchGuardrailAuditEventsRequest,
  ExportWorkbenchMemoriesRequest,
  GenerateWorkbenchSuggestionsRequest,
  GetWorkbenchArtifactContentRequest,
  GetWorkbenchArtifactSignedURLRequest,
  GetWorkbenchRunRequest,
  GetWorkbenchThreadRequest,
  GetWorkbenchTokenUsageRequest,
  ImportWorkbenchMemoriesRequest,
  ListWorkbenchArtifactsRequest,
  ListWorkbenchArtifactScanJobsRequest,
  ListWorkbenchGuardrailAuditEventsRequest,
  ListWorkbenchMCPRuntimeAuditEventsRequest,
  ListWorkbenchMemoriesRequest,
  ListWorkbenchMemoryAuditEventsRequest,
  ListWorkbenchMessagesRequest,
  ListWorkbenchRunEventsRequest,
  ListWorkbenchRunsRequest,
  ListWorkbenchUploadsRequest,
  ResumeWorkbenchRunRequest,
  RetryWorkbenchArtifactScanJobRequest,
  RetryWorkbenchSubagentRunRequest,
  ReviewWorkbenchArtifactScanRequest,
  SearchWorkbenchThreadsRequest,
  SubscribeWorkbenchRunEventsRequest,
  UpdateWorkbenchMemoryRequest,
  UploadWorkbenchFilesRequest,
  WorkbenchArtifactRequest,
  WorkbenchMemoryImportItem,
  WorkbenchMemoryRequest,
  WorkbenchThreadClient,
} from './workbench-thread-client';
import {
  createRunEventCursorStore,
  greatestRunEventCursor,
  type RunEventCursorScope,
  type RunEventCursorStore,
} from './run-event-cursor';
import {
  createWorkbenchClientOperationTracker,
  observeWorkbenchClientOperation,
  workbenchClientIdentifiers,
  workbenchClientOperations,
  type WorkbenchClientOperation,
  type WorkbenchClientTelemetry,
} from './client-telemetry';
import {
  canonicalErrorFromResponse,
  fetchCanonicalBlob,
  fetchCanonicalJSON,
  invalidCanonicalRequest,
  invalidCanonicalResponse,
  WorkbenchClientError,
  type CanonicalFetch,
} from './canonical-fetch';
import {
  adaptCanonicalArtifactContent,
  adaptCanonicalArtifactPage,
  adaptCanonicalArtifactRestore,
  adaptCanonicalArtifactScanJobPage,
  adaptCanonicalArtifactScanRetry,
  adaptCanonicalArtifactScanReview,
  adaptCanonicalArtifactSignedURL,
  adaptCanonicalGuardrailAuditExport,
  adaptCanonicalGuardrailAuditPage,
  adaptCanonicalMCPRuntimeAuditPage,
  adaptCanonicalMemoryAuditPage,
  adaptCanonicalMemoryClear,
  adaptCanonicalMemoryExport,
  adaptCanonicalMemoryImport,
  adaptCanonicalMemoryPage,
  adaptCanonicalMemoryRestore,
  adaptCanonicalMemoryUpdate,
  adaptCanonicalMessage,
  adaptCanonicalMessagePage,
  adaptCanonicalRun,
  adaptCanonicalRunCreation,
  adaptCanonicalRunEvent,
  adaptCanonicalRunEventPage,
  adaptCanonicalRunList,
  adaptCanonicalThread,
  adaptCanonicalThreadCreation,
  adaptCanonicalThreadList,
  adaptCanonicalSuggestions,
  adaptCanonicalTokenUsageResult,
  adaptCanonicalUploadCreation,
  adaptCanonicalUploadPage,
  assertCanonicalRequestResourceID,
  parseCanonicalWriteJSON,
  parseCanonicalWriteJSONObject,
  parseRequiredCanonicalWriteJSONObject,
} from './adapters/canonical-thread-adapter';

export type CanonicalWorkbenchCoreClient = Pick<
  WorkbenchThreadClient,
  | 'contract'
  | 'searchThreads'
  | 'createThread'
  | 'getThread'
  | 'listMessages'
  | 'listRuns'
  | 'createRun'
  | 'getRun'
  | 'cancelRun'
  | 'resumeRun'
  | 'listRunEvents'
>;

export type CanonicalWorkbenchProductClient = WorkbenchThreadClient;

export type CanonicalFetchStream = typeof fetchStream;

export interface CanonicalThreadCoreClientOptions {
  fetch?: CanonicalFetch;
  stream?: CanonicalFetchStream;
  cursorStore?: RunEventCursorStore;
  telemetry?: WorkbenchClientTelemetry;
  now?: () => number;
}

export type CanonicalThreadClientOptions = CanonicalThreadCoreClientOptions;

const canonicalPublicAssistantID = 'agent';
const defaultPageSize = 20;
const coreMaximumPageSize = 100;
const productMaximumPageSize = 200;
const maxCanonicalPageValue = 2_147_483_647;
const nonNegativeDecimal = /^(0|[1-9]\d*)$/;

const requiredBody = (body: unknown | undefined): unknown => {
  if (body === undefined) {
    throw invalidCanonicalResponse('Canonical success response body is empty');
  }
  return body;
};

const asPageValue = (
  value: number | undefined,
  fallback: number,
  label: string,
): number => {
  const candidate = value ?? fallback;
  if (
    !Number.isSafeInteger(candidate) ||
    candidate <= 0 ||
    candidate > maxCanonicalPageValue
  ) {
    throw invalidCanonicalRequest(
      'invalid_pagination',
      `${label} must be a positive integer`,
    );
  }
  return candidate;
};

const offsetPagination = (
  page: number | undefined,
  pageSize: number | undefined,
  maximumPageSize: number,
) => {
  const resolvedPage = asPageValue(page, 1, 'page');
  const requestedLimit = asPageValue(pageSize, defaultPageSize, 'page_size');
  const limit = Math.min(requestedLimit, maximumPageSize);
  const offset = (resolvedPage - 1) * limit;
  if (!Number.isSafeInteger(offset) || offset > maxCanonicalPageValue) {
    throw invalidCanonicalRequest(
      'invalid_pagination',
      'page and page_size produce an unsupported offset',
    );
  }
  return { limit, offset };
};

const coreOffsetPagination = (page?: number, pageSize?: number) =>
  offsetPagination(page, pageSize, coreMaximumPageSize);

const productOffsetPagination = (page?: number, pageSize?: number) =>
  offsetPagination(page, pageSize, productMaximumPageSize);

const cursorLimit = (value?: number): number =>
  asPageValue(value, defaultPageSize, 'limit');

const optionalTrimmed = (value: string | undefined): string | undefined => {
  const trimmed = value?.trim();
  return trimmed ? trimmed : undefined;
};

const requestAssistantID = (value: string | undefined): string => {
  const assistantID = optionalTrimmed(value) ?? canonicalPublicAssistantID;
  if (assistantID !== canonicalPublicAssistantID) {
    throw invalidCanonicalRequest(
      'invalid_assistant_id',
      'assistant_id must use the public alias agent',
    );
  }
  return assistantID;
};

const unsupportedThreadCreateOptions = (
  request: CreateWorkbenchThreadRequest,
): void => {
  const options = [
    ['command', request.command],
    ['stream_mode', request.stream_mode],
    ['multitask_strategy', request.multitask_strategy],
    ['on_disconnect', request.on_disconnect],
    ['durability', request.durability],
  ] as const;
  const unsupported = options.find(([, value]) => optionalTrimmed(value));
  if (unsupported) {
    throw invalidCanonicalRequest(
      'unsupported_create_option',
      `${unsupported[0]} is not supported by canonical Thread creation`,
    );
  }
};

const asNonEmptyRequestString = (value: unknown, label: string): string => {
  if (typeof value !== 'string' || value.trim() === '') {
    throw invalidCanonicalRequest(
      'invalid_request_shape',
      `${label} must be a non-empty string`,
    );
  }
  return value.trim();
};

const canonicalUploadedFiles = (
  input: Record<string, unknown>,
): Array<{ file_id: string }> => {
  const value = input.uploaded_files;
  if (value === undefined || value === null) {
    return [];
  }
  if (!Array.isArray(value)) {
    throw invalidCanonicalRequest(
      'invalid_request_shape',
      'input.uploaded_files must be an array',
    );
  }
  const seen = new Set<string>();
  return value.map((item, index) => {
    if (typeof item !== 'object' || item === null || Array.isArray(item)) {
      throw invalidCanonicalRequest(
        'invalid_request_shape',
        `input.uploaded_files[${index}] must be an object`,
      );
    }
    const fileID = assertCanonicalRequestResourceID(
      (item as Record<string, unknown>).file_id,
      `input.uploaded_files[${index}].file_id`,
    );
    if (seen.has(fileID)) {
      throw invalidCanonicalRequest(
        'invalid_uploaded_files',
        'input.uploaded_files must not contain duplicate file IDs',
      );
    }
    seen.add(fileID);
    return { file_id: fileID };
  });
};

const canonicalStreamMode = (
  value: string | undefined,
): string | string[] | undefined => {
  const trimmed = optionalTrimmed(value);
  if (trimmed === undefined) {
    return undefined;
  }
  if (!trimmed.startsWith('[')) {
    return trimmed;
  }
  const parsed = parseCanonicalWriteJSON(trimmed, 'stream_mode');
  if (
    !Array.isArray(parsed) ||
    parsed.length === 0 ||
    parsed.some(mode => typeof mode !== 'string' || mode.trim() === '')
  ) {
    throw invalidCanonicalRequest(
      'invalid_request_shape',
      'stream_mode must contain a non-empty string array',
    );
  }
  return parsed.map(mode => (mode as string).trim());
};

const canonicalRunCozeExtension = (
  request: CreateWorkbenchRunRequest,
  messageMetadata: Record<string, unknown> | undefined,
): Record<string, unknown> | undefined => {
  const attemptKind = optionalTrimmed(request.attempt_kind) ?? 'turn';
  const sourceRunID = optionalTrimmed(request.source_run_id);
  if (attemptKind === 'retry') {
    if (messageMetadata !== undefined) {
      throw invalidCanonicalRequest(
        'invalid_retry',
        'Top-level retry cannot create Message metadata',
      );
    }
    if (sourceRunID === undefined) {
      throw invalidCanonicalRequest(
        'invalid_retry',
        'Top-level retry requires source_run_id',
      );
    }
    return {
      attempt_kind: 'retry',
      source_run_id: assertCanonicalRequestResourceID(
        sourceRunID,
        'source_run_id',
      ),
    };
  }
  if (attemptKind !== 'turn') {
    throw invalidCanonicalRequest(
      'invalid_retry',
      'attempt_kind must be turn or retry',
    );
  }
  if (sourceRunID !== undefined) {
    throw invalidCanonicalRequest(
      'invalid_retry',
      'source_run_id is only valid for top-level retry',
    );
  }
  return messageMetadata === undefined
    ? undefined
    : { message_metadata: messageMetadata };
};

const canonicalResumeResponse = (
  response: ResumeWorkbenchRunRequest['response'],
) => {
  const schema = asNonEmptyRequestString(response.schema, 'response.schema');
  const interactionID = asNonEmptyRequestString(
    response.interaction_id,
    'response.interaction_id',
  );
  const kind = asNonEmptyRequestString(response.kind, 'response.kind');
  const decision = asNonEmptyRequestString(
    response.decision,
    'response.decision',
  );
  const optional = (value: unknown, label: string): string | undefined => {
    if (value === undefined) {
      return undefined;
    }
    if (typeof value !== 'string') {
      throw invalidCanonicalRequest(
        'invalid_request_shape',
        `${label} must be a string`,
      );
    }
    return value;
  };
  const answer = optional(response.answer, 'response.answer');
  const choiceID = optional(response.choice_id, 'response.choice_id');
  const comment = optional(response.comment, 'response.comment');

  return {
    schema,
    interaction_id: interactionID,
    kind,
    decision,
    ...(answer === undefined ? {} : { answer }),
    ...(choiceID === undefined ? {} : { choice_id: choiceID }),
    ...(comment === undefined ? {} : { comment }),
  };
};

const canonicalThreadScope = (request: {
  space_id: string;
  thread_id: string;
}) => ({
  spaceID: assertCanonicalRequestResourceID(request.space_id, 'space_id'),
  threadID: assertCanonicalRequestResourceID(request.thread_id, 'thread_id'),
});

const optionalPositiveInteger = (
  value: number | undefined,
  label: string,
): number | undefined => {
  if (value === undefined) {
    return undefined;
  }
  return asPageValue(value, value, label);
};

const optionalNonNegativeInteger = (
  value: number | undefined,
  label: string,
): number | undefined => {
  if (value === undefined) {
    return undefined;
  }
  if (
    !Number.isSafeInteger(value) ||
    value < 0 ||
    value > maxCanonicalPageValue
  ) {
    throw invalidCanonicalRequest(
      'invalid_request_shape',
      `${label} must be a non-negative integer`,
    );
  }
  return value;
};

const optionalFiniteNumber = (
  value: number | undefined,
  label: string,
): number | undefined => {
  if (value === undefined) {
    return undefined;
  }
  if (!Number.isFinite(value)) {
    throw invalidCanonicalRequest(
      'invalid_request_shape',
      `${label} must be a finite number`,
    );
  }
  return value;
};

const optionalCanonicalTime = (
  value: number | undefined,
  label: string,
): string | undefined => {
  if (value === undefined) {
    return undefined;
  }
  if (!Number.isSafeInteger(value) || value < 0) {
    throw invalidCanonicalRequest(
      'invalid_time',
      `${label} must be a non-negative epoch millisecond integer`,
    );
  }
  const date = new Date(value);
  if (!Number.isFinite(date.getTime())) {
    throw invalidCanonicalRequest('invalid_time', `${label} is out of range`);
  }
  return date.toISOString();
};

const setOptionalResourceID = (
  query: URLSearchParams,
  name: string,
  value: string | undefined,
): void => {
  if (value !== undefined) {
    query.set(name, assertCanonicalRequestResourceID(value, name));
  }
};

const setOptionalString = (
  query: URLSearchParams,
  name: string,
  value: string | undefined,
): void => {
  const normalized = optionalTrimmed(value);
  if (normalized !== undefined) {
    query.set(name, normalized);
  }
};

const setOptionalBoolean = (
  query: URLSearchParams,
  name: string,
  value: boolean | undefined,
): void => {
  if (value !== undefined) {
    query.set(name, String(value));
  }
};

const appendOptionalStrings = (
  query: URLSearchParams,
  name: string,
  values: string[] | undefined,
): void => {
  values?.forEach((value, index) => {
    query.append(name, asNonEmptyRequestString(value, `${name}[${index}]`));
  });
};

const assertCanonicalEmptySuccess = (
  body: unknown | undefined,
  operation: string,
): void => {
  if (body !== undefined) {
    throw invalidCanonicalResponse(
      `Canonical ${operation} response must not contain a body`,
    );
  }
};

const canonicalArtifactMode = (
  value: 'preview' | 'download',
): 'preview' | 'download' => {
  if (value !== 'preview' && value !== 'download') {
    throw invalidCanonicalRequest(
      'invalid_artifact_mode',
      'mode must be preview or download',
    );
  }
  return value;
};

const canonicalMemoryMutation = (
  item: UpdateWorkbenchMemoryRequest | WorkbenchMemoryImportItem,
) => {
  const runID =
    item.run_id === undefined
      ? undefined
      : assertCanonicalRequestResourceID(item.run_id, 'run_id');
  const scope = optionalTrimmed(item.scope);
  const content = asNonEmptyRequestString(item.content, 'content');
  const metadata = parseCanonicalWriteJSONObject(item.metadata, 'metadata');
  const correctionID =
    item.correction_of_memory_id === undefined
      ? undefined
      : assertCanonicalRequestResourceID(
          item.correction_of_memory_id,
          'correction_of_memory_id',
        );
  const correctedAt = optionalCanonicalTime(item.corrected_at, 'corrected_at');
  const expiresAt = optionalCanonicalTime(item.expires_at, 'expires_at');
  const score = optionalFiniteNumber(item.score, 'score');
  const confidence = optionalFiniteNumber(item.confidence, 'confidence');
  const sourceType = optionalTrimmed(item.source_type);
  const sourceID = optionalTrimmed(item.source_id);
  return {
    ...(runID === undefined ? {} : { run_id: runID }),
    ...(scope === undefined ? {} : { scope }),
    content,
    ...(metadata === undefined ? {} : { metadata }),
    ...(score === undefined ? {} : { score }),
    ...(confidence === undefined ? {} : { confidence }),
    ...(sourceType === undefined ? {} : { source_type: sourceType }),
    ...(sourceID === undefined ? {} : { source_id: sourceID }),
    ...(correctionID === undefined
      ? {}
      : { correction_of_memory_id: correctionID }),
    ...(correctedAt === undefined ? {} : { corrected_at: correctedAt }),
    ...(expiresAt === undefined ? {} : { expires_at: expiresAt }),
  };
};

const appendMemoryQuery = (
  query: URLSearchParams,
  request: {
    run_id?: string;
    scope?: string;
    scopes?: string[];
    q?: string;
    include_expired?: boolean;
    include_deleted?: boolean;
  },
): void => {
  setOptionalResourceID(query, 'run_id', request.run_id);
  setOptionalString(query, 'scope', request.scope);
  appendOptionalStrings(query, 'scopes', request.scopes);
  setOptionalString(query, 'q', request.q);
  setOptionalBoolean(query, 'include_expired', request.include_expired);
  setOptionalBoolean(query, 'include_deleted', request.include_deleted);
};

type AsyncWorkbenchClientOperation = Exclude<
  WorkbenchClientOperation,
  'subscribeRunEvents'
>;

const asyncWorkbenchClientOperations = workbenchClientOperations.filter(
  (operation): operation is AsyncWorkbenchClientOperation =>
    operation !== 'subscribeRunEvents',
);

type AsyncWorkbenchClientMethod = (request: unknown) => Promise<unknown>;

type CanonicalRunStreamMessage =
  | {
      kind: 'event';
      event: ReturnType<typeof adaptCanonicalRunEvent>;
    }
  | { kind: 'end' }
  | { kind: 'error'; error: WorkbenchClientError };

const streamJSONRecord = (
  data: string,
  label: string,
): Record<string, unknown> => {
  let parsed: unknown;
  try {
    parsed = JSON.parse(data) as unknown;
  } catch (error) {
    void error;
    throw invalidCanonicalResponse(`${label} must contain valid JSON`);
  }
  if (typeof parsed !== 'object' || parsed === null || Array.isArray(parsed)) {
    throw invalidCanonicalResponse(`${label} must contain a JSON object`);
  }
  return parsed as Record<string, unknown>;
};

const canonicalRunStreamError = (data: string): WorkbenchClientError => {
  const payload = streamJSONRecord(data, 'Canonical Run stream error');
  if (
    typeof payload.code !== 'string' ||
    payload.code.trim() === '' ||
    typeof payload.message !== 'string' ||
    payload.message.trim() === ''
  ) {
    throw invalidCanonicalResponse(
      'Canonical Run stream error has invalid fields',
    );
  }
  return new WorkbenchClientError({
    message: payload.message,
    code: payload.code,
    retryable: false,
    outcome: 'failed',
  });
};

const canonicalRunPublicEvent = (
  frame: { id?: string; data: string },
  scope: RunEventCursorScope,
): CanonicalRunStreamMessage => {
  if (!frame.id || !/^[1-9]\d*$/.test(frame.id)) {
    throw invalidCanonicalResponse(
      'Canonical Run stream event ID must be a positive decimal',
    );
  }
  const event = adaptCanonicalRunEvent(
    streamJSONRecord(frame.data, 'Canonical Run stream event'),
    {
      spaceId: scope.spaceId,
      threadId: scope.threadId,
      runId: scope.runId,
    },
  );
  if (event.event_id !== frame.id) {
    throw invalidCanonicalResponse(
      'Canonical Run stream frame ID does not match event_id',
    );
  }
  return { kind: 'event', event };
};

const canonicalRunTerminalEvent = (
  data: string,
  scope: RunEventCursorScope,
): CanonicalRunStreamMessage => {
  const terminal = streamJSONRecord(
    data,
    'Canonical Run stream terminal event',
  );
  if (
    terminal.thread_id !== scope.threadId ||
    terminal.run_id !== scope.runId
  ) {
    throw invalidCanonicalResponse(
      'Canonical Run stream terminal IDs do not match the subscription',
    );
  }
  if (
    typeof terminal.status !== 'string' ||
    !['success', 'error', 'interrupted'].includes(terminal.status) ||
    typeof terminal.reason !== 'string' ||
    terminal.reason.trim() === ''
  ) {
    throw invalidCanonicalResponse(
      'Canonical Run stream terminal event has invalid fields',
    );
  }
  return { kind: 'end' };
};

const canonicalRunStreamMessage = (
  frame: { event?: string; id?: string; data: string },
  scope: RunEventCursorScope,
): CanonicalRunStreamMessage | undefined => {
  if (
    !frame.event ||
    frame.event === 'metadata' ||
    frame.event === 'heartbeat'
  ) {
    return undefined;
  }
  try {
    if (frame.event === 'events') {
      return canonicalRunPublicEvent(frame, scope);
    }
    if (frame.event === 'end') {
      return canonicalRunTerminalEvent(frame.data, scope);
    }
    if (frame.event === 'error') {
      return { kind: 'error', error: canonicalRunStreamError(frame.data) };
    }
    return undefined;
  } catch (error) {
    return {
      kind: 'error',
      error:
        error instanceof WorkbenchClientError
          ? error
          : invalidCanonicalResponse(
              'Canonical Run stream frame could not be projected',
            ),
    };
  }
};

const normalizeCanonicalStreamFailure = (
  error: unknown,
): WorkbenchClientError =>
  error instanceof WorkbenchClientError
    ? error
    : new WorkbenchClientError({
        message: 'Canonical Run stream failed',
        code: 'stream_transport_error',
        retryable: true,
        outcome: 'failed',
      });

type CanonicalRunStreamOutcome =
  | 'success'
  | 'error'
  | 'canceled'
  | 'disconnected';

interface CanonicalRunStreamLifecycleOptions {
  request: SubscribeWorkbenchRunEventsRequest;
  scope: RunEventCursorScope;
  cursorStore: RunEventCursorStore;
  tracker: ReturnType<typeof createWorkbenchClientOperationTracker>;
}

class CanonicalRunStreamLifecycle {
  readonly controller = new AbortController();
  readonly closed: Promise<void>;

  private resolveClosed: () => void = () => undefined;
  private finished = false;

  constructor(private readonly options: CanonicalRunStreamLifecycleOptions) {
    this.closed = new Promise<void>(resolve => {
      this.resolveClosed = resolve;
    });
    options.request.signal.addEventListener('abort', this.onAbort, {
      once: true,
    });
    if (options.request.signal.aborted) {
      this.finish('canceled');
    }
  }

  get isFinished(): boolean {
    return this.finished;
  }

  subscription() {
    return {
      close: () => this.finish('canceled'),
      closed: this.closed,
    };
  }

  handleMessage(message: CanonicalRunStreamMessage): void {
    if (this.finished) {
      return;
    }
    if (message.kind === 'event') {
      try {
        this.options.request.onEvent(message.event);
        this.options.cursorStore.write(
          this.options.scope,
          message.event.event_id,
        );
      } catch (error) {
        this.finish('error', normalizeCanonicalStreamFailure(error));
      }
    } else if (message.kind === 'end') {
      this.finish('success');
    } else {
      this.finish('error', message.error);
    }
  }

  finish(
    outcome: CanonicalRunStreamOutcome,
    error?: WorkbenchClientError,
  ): void {
    if (this.finished) {
      return;
    }
    this.finished = true;
    const { request, tracker, cursorStore, scope } = this.options;
    request.signal.removeEventListener('abort', this.onAbort);
    if (!this.controller.signal.aborted) {
      this.controller.abort();
    }
    try {
      if (outcome === 'success') {
        tracker.success();
        cursorStore.clear(scope);
        request.onEnd();
      } else if (outcome === 'error' && error) {
        tracker.error(error);
        request.onError(error);
      } else if (outcome === 'disconnected' && error) {
        tracker.disconnected();
        request.onError(error);
      } else {
        tracker.canceled();
      }
    } finally {
      this.resolveClosed();
    }
  }

  private readonly onAbort = () => this.finish('canceled');
}

export class CanonicalThreadCoreClient
  implements CanonicalWorkbenchProductClient
{
  readonly contract = 'canonical_v1' as const;

  private readonly fetcher: CanonicalFetch | undefined;
  private readonly streamer: CanonicalFetchStream;
  private readonly cursorStore: RunEventCursorStore;
  private readonly telemetry: WorkbenchClientTelemetry | undefined;
  private readonly now: () => number;

  constructor(options: CanonicalThreadCoreClientOptions = {}) {
    this.fetcher = options.fetch;
    this.streamer = options.stream ?? fetchStream;
    this.cursorStore = options.cursorStore ?? createRunEventCursorStore();
    this.telemetry = options.telemetry;
    this.now = options.now ?? Date.now;
    this.installOperationTelemetry();
  }

  private installOperationTelemetry(): void {
    const methods = this as unknown as Record<
      AsyncWorkbenchClientOperation,
      AsyncWorkbenchClientMethod
    >;
    asyncWorkbenchClientOperations.forEach(operation => {
      const execute = methods[operation].bind(this);
      methods[operation] = request =>
        observeWorkbenchClientOperation({
          telemetry: this.telemetry,
          operation,
          identifiers: workbenchClientIdentifiers(request),
          now: this.now,
          execute: () => execute(request),
        });
    });
  }

  async searchThreads(request: SearchWorkbenchThreadsRequest) {
    const spaceID = assertCanonicalRequestResourceID(
      request.space_id,
      'space_id',
    );
    const { limit, offset } = coreOffsetPagination(
      request.page,
      request.page_size,
    );
    const status = optionalTrimmed(request.status);
    const result = await fetchCanonicalJSON('/api/workbench/threads/search', {
      fetch: this.fetcher,
      method: 'POST',
      spaceId: spaceID,
      json: {
        limit,
        offset,
        ...(status === undefined ? {} : { status }),
      },
      signal: request.signal,
    });
    return adaptCanonicalThreadList(
      requiredBody(result.body),
      spaceID,
      result.pagination,
    );
  }

  async createThread(request: CreateWorkbenchThreadRequest) {
    const spaceID = assertCanonicalRequestResourceID(
      request.space_id,
      'space_id',
    );
    unsupportedThreadCreateOptions(request);
    const assistantID = requestAssistantID(request.assistant_id);
    const config = parseCanonicalWriteJSONObject(request.config, 'config');
    const context = parseCanonicalWriteJSONObject(request.context, 'context');
    const metadata = parseCanonicalWriteJSONObject(
      request.metadata,
      'metadata',
    );

    const message = asNonEmptyRequestString(request.message, 'message');
    const title = optionalTrimmed(request.title);
    const initialRun = {
      assistant_id: assistantID,
      input: { messages: [{ role: 'user', content: message }] },
      ...(config === undefined ? {} : { config }),
      ...(context === undefined ? {} : { context }),
      ...(metadata === undefined ? {} : { metadata }),
    };
    const result = await fetchCanonicalJSON('/api/workbench/threads', {
      fetch: this.fetcher,
      method: 'POST',
      spaceId: spaceID,
      json: {
        metadata: {
          ...(title === undefined ? {} : { title }),
          source: 'web',
        },
        coze: request.defer_start
          ? { deferred_initial_run: initialRun }
          : { initial_run: initialRun },
      },
      idempotencyKey: request.idempotency_key,
      signal: request.signal,
    });
    return adaptCanonicalThreadCreation(requiredBody(result.body), spaceID);
  }

  async getThread(request: GetWorkbenchThreadRequest) {
    const spaceID = assertCanonicalRequestResourceID(
      request.space_id,
      'space_id',
    );
    const threadID = assertCanonicalRequestResourceID(
      request.thread_id,
      'thread_id',
    );
    const result = await fetchCanonicalJSON(
      `/api/workbench/threads/${threadID}`,
      {
        fetch: this.fetcher,
        method: 'GET',
        spaceId: spaceID,
        signal: request.signal,
      },
    );
    return adaptCanonicalThread(requiredBody(result.body), {
      spaceId: spaceID,
      threadId: threadID,
    });
  }

  async listMessages(request: ListWorkbenchMessagesRequest) {
    const spaceID = assertCanonicalRequestResourceID(
      request.space_id,
      'space_id',
    );
    const threadID = assertCanonicalRequestResourceID(
      request.thread_id,
      'thread_id',
    );
    if (request.before_seq !== undefined && request.after_seq !== undefined) {
      throw invalidCanonicalRequest(
        'invalid_message_cursor',
        'before_seq and after_seq are mutually exclusive',
      );
    }
    const query = new URLSearchParams();
    query.set('limit', String(cursorLimit(request.limit)));
    if (request.before_seq !== undefined) {
      query.set(
        'before_seq',
        assertCanonicalRequestResourceID(request.before_seq, 'before_seq'),
      );
    }
    if (request.after_seq !== undefined) {
      query.set(
        'after_seq',
        assertCanonicalRequestResourceID(request.after_seq, 'after_seq'),
      );
    }
    const result = await fetchCanonicalJSON(
      `/api/workbench/threads/${threadID}/messages?${query.toString()}`,
      {
        fetch: this.fetcher,
        method: 'GET',
        spaceId: spaceID,
        signal: request.signal,
      },
    );
    return adaptCanonicalMessagePage(requiredBody(result.body), {
      spaceId: spaceID,
      threadId: threadID,
    });
  }

  async listRuns(request: ListWorkbenchRunsRequest) {
    const spaceID = assertCanonicalRequestResourceID(
      request.space_id,
      'space_id',
    );
    const threadID = assertCanonicalRequestResourceID(
      request.thread_id,
      'thread_id',
    );
    const { limit, offset } = coreOffsetPagination(
      request.page,
      request.page_size,
    );
    const query = new URLSearchParams();
    if (request.parent_run_id !== undefined) {
      query.set(
        'parent_run_id',
        assertCanonicalRequestResourceID(
          request.parent_run_id,
          'parent_run_id',
        ),
      );
    }
    const status = optionalTrimmed(request.status);
    if (status !== undefined) {
      query.set('status', status);
    }
    query.set('limit', String(limit));
    query.set('offset', String(offset));
    const result = await fetchCanonicalJSON(
      `/api/workbench/threads/${threadID}/runs?${query.toString()}`,
      {
        fetch: this.fetcher,
        method: 'GET',
        spaceId: spaceID,
        signal: request.signal,
      },
    );
    return adaptCanonicalRunList(
      requiredBody(result.body),
      { spaceId: spaceID, threadId: threadID },
      result.pagination,
    );
  }

  async createRun(request: CreateWorkbenchRunRequest) {
    const spaceID = assertCanonicalRequestResourceID(
      request.space_id,
      'space_id',
    );
    const threadID = assertCanonicalRequestResourceID(
      request.thread_id,
      'thread_id',
    );
    const assistantID = requestAssistantID(request.assistant_id);
    const input = parseRequiredCanonicalWriteJSONObject(request.input, 'input');
    const command = parseCanonicalWriteJSONObject(request.command, 'command');
    const config = parseCanonicalWriteJSONObject(request.config, 'config');
    const context = parseCanonicalWriteJSONObject(request.context, 'context');
    const metadata = parseCanonicalWriteJSONObject(
      request.metadata,
      'metadata',
    );
    const messageMetadata = parseCanonicalWriteJSONObject(
      request.message_metadata,
      'message_metadata',
    );
    const coze = canonicalRunCozeExtension(request, messageMetadata);
    const messageContent = asNonEmptyRequestString(
      request.message_content,
      'message_content',
    );
    const streamMode = canonicalStreamMode(request.stream_mode);
    const body = {
      assistant_id: assistantID,
      input: {
        messages: [{ role: 'user', content: messageContent }],
        uploaded_files: canonicalUploadedFiles(input),
      },
      ...(command === undefined ? {} : { command }),
      ...(config === undefined ? {} : { config }),
      ...(context === undefined ? {} : { context }),
      ...(metadata === undefined ? {} : { metadata }),
      ...(coze === undefined ? {} : { coze }),
      ...(streamMode === undefined ? {} : { stream_mode: streamMode }),
      ...(optionalTrimmed(request.multitask_strategy) === undefined
        ? {}
        : { multitask_strategy: optionalTrimmed(request.multitask_strategy) }),
      ...(optionalTrimmed(request.on_disconnect) === undefined
        ? {}
        : { on_disconnect: optionalTrimmed(request.on_disconnect) }),
      ...(optionalTrimmed(request.durability) === undefined
        ? {}
        : { durability: optionalTrimmed(request.durability) }),
    };
    const result = await fetchCanonicalJSON(
      `/api/workbench/threads/${threadID}/runs`,
      {
        fetch: this.fetcher,
        method: 'POST',
        spaceId: spaceID,
        json: body,
        idempotencyKey: request.idempotency_key,
        signal: request.signal,
      },
    );
    return adaptCanonicalRunCreation(requiredBody(result.body), {
      spaceId: spaceID,
      threadId: threadID,
    });
  }

  async getRun(request: GetWorkbenchRunRequest) {
    const spaceID = assertCanonicalRequestResourceID(
      request.space_id,
      'space_id',
    );
    const threadID = assertCanonicalRequestResourceID(
      request.thread_id,
      'thread_id',
    );
    const runID = assertCanonicalRequestResourceID(request.run_id, 'run_id');
    const result = await fetchCanonicalJSON(
      `/api/workbench/threads/${threadID}/runs/${runID}`,
      {
        fetch: this.fetcher,
        method: 'GET',
        spaceId: spaceID,
        signal: request.signal,
      },
    );
    return adaptCanonicalRun(requiredBody(result.body), {
      spaceId: spaceID,
      threadId: threadID,
      runId: runID,
    });
  }

  async cancelRun(request: CancelWorkbenchRunRequest): Promise<void> {
    const spaceID = assertCanonicalRequestResourceID(
      request.space_id,
      'space_id',
    );
    const threadID = assertCanonicalRequestResourceID(
      request.thread_id,
      'thread_id',
    );
    const runID = assertCanonicalRequestResourceID(request.run_id, 'run_id');
    const result = await fetchCanonicalJSON(
      `/api/workbench/threads/${threadID}/runs/${runID}/cancel`,
      {
        fetch: this.fetcher,
        method: 'POST',
        spaceId: spaceID,
        signal: request.signal,
      },
    );
    if (result.body !== undefined) {
      throw invalidCanonicalResponse(
        'Canonical cancel response must not contain a body',
      );
    }
  }

  async resumeRun(request: ResumeWorkbenchRunRequest) {
    const spaceID = assertCanonicalRequestResourceID(
      request.space_id,
      'space_id',
    );
    const threadID = assertCanonicalRequestResourceID(
      request.thread_id,
      'thread_id',
    );
    const runID = assertCanonicalRequestResourceID(request.run_id, 'run_id');
    const interruptID = asNonEmptyRequestString(
      request.interrupt_id,
      'interrupt_id',
    );
    const result = await fetchCanonicalJSON(
      `/api/workbench/threads/${threadID}/runs/${runID}/resume`,
      {
        fetch: this.fetcher,
        method: 'POST',
        spaceId: spaceID,
        json: {
          interrupt_id: interruptID,
          response: canonicalResumeResponse(request.response),
        },
        idempotencyKey: request.idempotency_key,
        signal: request.signal,
      },
    );
    return adaptCanonicalRun(requiredBody(result.body), {
      spaceId: spaceID,
      threadId: threadID,
    });
  }

  async listRunEvents(request: ListWorkbenchRunEventsRequest) {
    const spaceID = assertCanonicalRequestResourceID(
      request.space_id,
      'space_id',
    );
    const threadID = assertCanonicalRequestResourceID(
      request.thread_id,
      'thread_id',
    );
    const runID = assertCanonicalRequestResourceID(request.run_id, 'run_id');
    const query = new URLSearchParams();
    if (request.cursor !== undefined) {
      if (!nonNegativeDecimal.test(request.cursor)) {
        throw invalidCanonicalRequest(
          'invalid_event_cursor',
          'cursor must be a non-negative decimal event ID',
        );
      }
      query.set('after_event_id', request.cursor);
    }
    query.set('limit', String(cursorLimit(request.limit)));
    request.event_types?.forEach((eventType, index) => {
      const value = asNonEmptyRequestString(eventType, `event_types[${index}]`);
      query.append('event_types', value);
    });
    const result = await fetchCanonicalJSON(
      `/api/workbench/threads/${threadID}/runs/${runID}/events?${query.toString()}`,
      {
        fetch: this.fetcher,
        method: 'GET',
        spaceId: spaceID,
        signal: request.signal,
      },
    );
    return adaptCanonicalRunEventPage(requiredBody(result.body), {
      spaceId: spaceID,
      threadId: threadID,
      runId: runID,
    });
  }

  subscribeRunEvents(request: SubscribeWorkbenchRunEventsRequest) {
    const spaceID = assertCanonicalRequestResourceID(
      request.space_id,
      'space_id',
    );
    const threadID = assertCanonicalRequestResourceID(
      request.thread_id,
      'thread_id',
    );
    const runID = assertCanonicalRequestResourceID(request.run_id, 'run_id');
    if (
      request.cursor !== undefined &&
      !nonNegativeDecimal.test(request.cursor)
    ) {
      throw invalidCanonicalRequest(
        'invalid_event_cursor',
        'cursor must be a non-negative decimal event ID',
      );
    }
    const scope: RunEventCursorScope = {
      contract: this.contract,
      spaceId: spaceID,
      threadId: threadID,
      runId: runID,
    };
    const cursor = greatestRunEventCursor(
      request.cursor,
      this.cursorStore.read(scope),
    );
    const query = new URLSearchParams();
    if (cursor !== undefined) {
      query.set('after_event_id', cursor);
    }
    query.set('cancel_on_disconnect', 'false');
    query.set('stream_mode', 'events');
    const url = `/api/workbench/threads/${threadID}/runs/${runID}/stream?${query.toString()}`;
    const tracker = createWorkbenchClientOperationTracker({
      telemetry: this.telemetry,
      operation: 'subscribeRunEvents',
      identifiers: { thread_id: threadID, run_id: runID },
      now: this.now,
    });
    const lifecycle = new CanonicalRunStreamLifecycle({
      request,
      scope,
      cursorStore: this.cursorStore,
      tracker,
    });
    const subscription = lifecycle.subscription();
    if (lifecycle.isFinished) {
      return subscription;
    }

    void this.streamer<CanonicalRunStreamMessage>(url, {
      method: 'GET',
      credentials: 'same-origin',
      headers: {
        Accept: 'text/event-stream',
        'X-Coze-Space-ID': spaceID,
        'x-requested-with': 'XMLHttpRequest',
      },
      signal: lifecycle.controller.signal,
      onStart: async response => {
        if (!response.ok) {
          throw await canonicalErrorFromResponse(response);
        }
        const contentType = response.headers.get('content-type');
        const mediaType = contentType?.split(';', 1)[0].trim().toLowerCase();
        if (mediaType !== 'text/event-stream') {
          throw invalidCanonicalResponse(
            'Canonical Run stream response must be text/event-stream',
            response.status,
          );
        }
      },
      streamParser: frame => canonicalRunStreamMessage(frame, scope),
      onMessage: ({ message }) => lifecycle.handleMessage(message),
      onAllSuccess: () => {
        lifecycle.finish(
          'disconnected',
          new WorkbenchClientError({
            message: 'Canonical Run stream disconnected before terminal end',
            code: 'stream_disconnected',
            retryable: true,
            outcome: 'failed',
          }),
        );
      },
      onError: ({ fetchStreamError }) => {
        lifecycle.finish(
          'error',
          normalizeCanonicalStreamFailure(fetchStreamError.error),
        );
      },
    }).catch(error => {
      lifecycle.finish('error', normalizeCanonicalStreamFailure(error));
    });

    return subscription;
  }

  async appendMessage(request: AppendWorkbenchMessageRequest) {
    const { spaceID, threadID } = canonicalThreadScope(request);
    const role = asNonEmptyRequestString(request.role, 'role').toLowerCase();
    if (role === 'user' || role === 'human') {
      throw invalidCanonicalRequest(
        'atomic_run_submission_required',
        'User messages must be submitted atomically through createRun',
      );
    }
    if (role !== 'assistant' && role !== 'tool') {
      throw invalidCanonicalRequest(
        'invalid_message_role',
        'role must be assistant or tool',
      );
    }
    if (request.run_id === undefined) {
      throw invalidCanonicalRequest(
        'invalid_run_id',
        'run_id is required for compatibility append',
      );
    }
    const runID = assertCanonicalRequestResourceID(request.run_id, 'run_id');
    const metadata = parseCanonicalWriteJSONObject(
      request.metadata,
      'metadata',
    );
    const result = await fetchCanonicalJSON(
      `/api/workbench/threads/${threadID}/messages`,
      {
        fetch: this.fetcher,
        method: 'POST',
        spaceId: spaceID,
        json: {
          run_id: runID,
          role,
          content: asNonEmptyRequestString(request.content, 'content'),
          ...(metadata === undefined ? {} : { metadata }),
          append_mode: 'internal_compat',
        },
        signal: request.signal,
      },
    );
    return adaptCanonicalMessage(requiredBody(result.body), {
      spaceId: spaceID,
      threadId: threadID,
      runId: runID,
    });
  }

  async generateSuggestions(request: GenerateWorkbenchSuggestionsRequest) {
    const { spaceID, threadID } = canonicalThreadScope(request);
    const count = optionalNonNegativeInteger(request.n, 'n');
    const modelName = optionalTrimmed(request.model_name);
    const modelType = optionalTrimmed(request.model_type);
    if (modelType !== undefined && !nonNegativeDecimal.test(modelType)) {
      throw invalidCanonicalRequest(
        'invalid_model_type',
        'model_type must be a non-negative decimal string',
      );
    }
    const result = await fetchCanonicalJSON(
      `/api/workbench/threads/${threadID}/suggestions`,
      {
        fetch: this.fetcher,
        method: 'POST',
        spaceId: spaceID,
        json: {
          ...(count === undefined ? {} : { n: count }),
          ...(modelName === undefined ? {} : { model_name: modelName }),
          ...(modelType === undefined ? {} : { model_type: modelType }),
        },
        signal: request.signal,
      },
    );
    return adaptCanonicalSuggestions(requiredBody(result.body));
  }

  async retrySubagentRun(request: RetryWorkbenchSubagentRunRequest) {
    const { spaceID, threadID } = canonicalThreadScope(request);
    const runID = assertCanonicalRequestResourceID(request.run_id, 'run_id');
    const result = await fetchCanonicalJSON(
      `/api/workbench/threads/${threadID}/runs/${runID}/retry`,
      {
        fetch: this.fetcher,
        method: 'POST',
        spaceId: spaceID,
        idempotencyKey: request.idempotency_key,
        signal: request.signal,
      },
    );
    return adaptCanonicalRun(requiredBody(result.body), {
      spaceId: spaceID,
      threadId: threadID,
    });
  }

  async listUploads(request: ListWorkbenchUploadsRequest) {
    const { spaceID, threadID } = canonicalThreadScope(request);
    const result = await fetchCanonicalJSON(
      `/api/workbench/threads/${threadID}/uploads`,
      {
        fetch: this.fetcher,
        method: 'GET',
        spaceId: spaceID,
        signal: request.signal,
      },
    );
    return adaptCanonicalUploadPage(requiredBody(result.body));
  }

  async uploadFiles(request: UploadWorkbenchFilesRequest) {
    const { spaceID, threadID } = canonicalThreadScope(request);
    if (!Array.isArray(request.files) || request.files.length === 0) {
      throw invalidCanonicalRequest(
        'invalid_upload',
        'files must contain at least one File',
      );
    }
    const form = new FormData();
    request.files.forEach((file, index) => {
      if (!(file instanceof File)) {
        throw invalidCanonicalRequest(
          'invalid_upload',
          `files[${index}] must be a File`,
        );
      }
      form.append('files', file);
    });
    const result = await fetchCanonicalJSON(
      `/api/workbench/threads/${threadID}/uploads`,
      {
        fetch: this.fetcher,
        method: 'POST',
        spaceId: spaceID,
        body: form,
        signal: request.signal,
      },
    );
    return adaptCanonicalUploadCreation(requiredBody(result.body));
  }

  async deleteUpload(request: DeleteWorkbenchUploadRequest): Promise<void> {
    const { spaceID, threadID } = canonicalThreadScope(request);
    const fileID = assertCanonicalRequestResourceID(request.file_id, 'file_id');
    const result = await fetchCanonicalJSON(
      `/api/workbench/threads/${threadID}/uploads/${fileID}`,
      {
        fetch: this.fetcher,
        method: 'DELETE',
        spaceId: spaceID,
        signal: request.signal,
      },
    );
    assertCanonicalEmptySuccess(result.body, 'Upload delete');
  }

  async listArtifacts(request: ListWorkbenchArtifactsRequest) {
    const { spaceID, threadID } = canonicalThreadScope(request);
    const { limit, offset } = productOffsetPagination(
      request.page,
      request.page_size,
    );
    const query = new URLSearchParams();
    setOptionalResourceID(query, 'run_id', request.run_id);
    setOptionalBoolean(query, 'deleted_only', request.deleted_only);
    query.set('limit', String(limit));
    query.set('offset', String(offset));
    const result = await fetchCanonicalJSON(
      `/api/workbench/threads/${threadID}/artifacts?${query.toString()}`,
      {
        fetch: this.fetcher,
        method: 'GET',
        spaceId: spaceID,
        signal: request.signal,
      },
    );
    return adaptCanonicalArtifactPage(requiredBody(result.body), {
      spaceId: spaceID,
      threadId: threadID,
    });
  }

  async getArtifactContent(request: GetWorkbenchArtifactContentRequest) {
    const { spaceID, threadID } = canonicalThreadScope(request);
    const artifactID = assertCanonicalRequestResourceID(
      request.artifact_id,
      'artifact_id',
    );
    const mode = canonicalArtifactMode(request.mode);
    const result = await fetchCanonicalBlob(
      `/api/workbench/threads/${threadID}/artifacts/${artifactID}/content?mode=${mode}`,
      {
        fetch: this.fetcher,
        method: 'GET',
        spaceId: spaceID,
        signal: request.signal,
      },
    );
    return adaptCanonicalArtifactContent(result);
  }

  async getArtifactSignedURL(request: GetWorkbenchArtifactSignedURLRequest) {
    const { spaceID, threadID } = canonicalThreadScope(request);
    const artifactID = assertCanonicalRequestResourceID(
      request.artifact_id,
      'artifact_id',
    );
    const query = new URLSearchParams();
    query.set('mode', canonicalArtifactMode(request.mode));
    const ttlSeconds = optionalNonNegativeInteger(
      request.ttl_seconds,
      'ttl_seconds',
    );
    if (ttlSeconds !== undefined) {
      query.set('ttl_seconds', String(ttlSeconds));
    }
    const result = await fetchCanonicalJSON(
      `/api/workbench/threads/${threadID}/artifacts/${artifactID}/signed_url?${query.toString()}`,
      {
        fetch: this.fetcher,
        method: 'GET',
        spaceId: spaceID,
        signal: request.signal,
      },
    );
    return adaptCanonicalArtifactSignedURL(
      requiredBody(result.body),
      artifactID,
    );
  }

  async deleteArtifact(request: WorkbenchArtifactRequest): Promise<void> {
    const { spaceID, threadID } = canonicalThreadScope(request);
    const artifactID = assertCanonicalRequestResourceID(
      request.artifact_id,
      'artifact_id',
    );
    const result = await fetchCanonicalJSON(
      `/api/workbench/threads/${threadID}/artifacts/${artifactID}`,
      {
        fetch: this.fetcher,
        method: 'DELETE',
        spaceId: spaceID,
        signal: request.signal,
      },
    );
    assertCanonicalEmptySuccess(result.body, 'Artifact delete');
  }

  async restoreArtifact(request: WorkbenchArtifactRequest) {
    const { spaceID, threadID } = canonicalThreadScope(request);
    const artifactID = assertCanonicalRequestResourceID(
      request.artifact_id,
      'artifact_id',
    );
    const result = await fetchCanonicalJSON(
      `/api/workbench/threads/${threadID}/artifacts/${artifactID}/restore`,
      {
        fetch: this.fetcher,
        method: 'POST',
        spaceId: spaceID,
        signal: request.signal,
      },
    );
    return adaptCanonicalArtifactRestore(
      requiredBody(result.body),
      { spaceId: spaceID, threadId: threadID },
      artifactID,
    );
  }

  async reviewArtifactScan(request: ReviewWorkbenchArtifactScanRequest) {
    const { spaceID, threadID } = canonicalThreadScope(request);
    const artifactID = assertCanonicalRequestResourceID(
      request.artifact_id,
      'artifact_id',
    );
    if (!['release', 'quarantine', 'block'].includes(request.decision)) {
      throw invalidCanonicalRequest(
        'invalid_scan_decision',
        'decision must be release, quarantine, or block',
      );
    }
    const reason = optionalTrimmed(request.reason);
    const result = await fetchCanonicalJSON(
      `/api/workbench/threads/${threadID}/artifacts/${artifactID}/scan_review`,
      {
        fetch: this.fetcher,
        method: 'POST',
        spaceId: spaceID,
        json: {
          decision: request.decision,
          ...(reason === undefined ? {} : { reason }),
        },
        signal: request.signal,
      },
    );
    return adaptCanonicalArtifactScanReview(
      requiredBody(result.body),
      artifactID,
    );
  }

  async listArtifactScanJobs(request: ListWorkbenchArtifactScanJobsRequest) {
    const { spaceID, threadID } = canonicalThreadScope(request);
    const { limit, offset } = productOffsetPagination(
      request.page,
      request.page_size,
    );
    const query = new URLSearchParams();
    setOptionalResourceID(query, 'run_id', request.run_id);
    setOptionalResourceID(query, 'artifact_id', request.artifact_id);
    setOptionalString(query, 'status', request.status);
    setOptionalString(query, 'scanner', request.scanner);
    query.set('limit', String(limit));
    query.set('offset', String(offset));
    const result = await fetchCanonicalJSON(
      `/api/workbench/threads/${threadID}/artifact_scan_jobs?${query.toString()}`,
      {
        fetch: this.fetcher,
        method: 'GET',
        spaceId: spaceID,
        signal: request.signal,
      },
    );
    return adaptCanonicalArtifactScanJobPage(requiredBody(result.body), {
      spaceId: spaceID,
      threadId: threadID,
    });
  }

  async retryArtifactScanJob(request: RetryWorkbenchArtifactScanJobRequest) {
    const { spaceID, threadID } = canonicalThreadScope(request);
    const jobID = assertCanonicalRequestResourceID(request.job_id, 'job_id');
    const result = await fetchCanonicalJSON(
      `/api/workbench/threads/${threadID}/artifact_scan_jobs/${jobID}/retry`,
      {
        fetch: this.fetcher,
        method: 'POST',
        spaceId: spaceID,
        signal: request.signal,
      },
    );
    return adaptCanonicalArtifactScanRetry(
      requiredBody(result.body),
      { spaceId: spaceID, threadId: threadID },
      jobID,
    );
  }

  async getTokenUsage(request: GetWorkbenchTokenUsageRequest) {
    const { spaceID, threadID } = canonicalThreadScope(request);
    const { limit, offset } = productOffsetPagination(
      request.page,
      request.page_size,
    );
    const query = new URLSearchParams();
    setOptionalResourceID(query, 'run_id', request.run_id);
    setOptionalBoolean(query, 'include_child_runs', request.include_child_runs);
    setOptionalString(query, 'source', request.source);
    query.set('limit', String(limit));
    query.set('offset', String(offset));
    const result = await fetchCanonicalJSON(
      `/api/workbench/threads/${threadID}/token_usage?${query.toString()}`,
      {
        fetch: this.fetcher,
        method: 'GET',
        spaceId: spaceID,
        signal: request.signal,
      },
    );
    return adaptCanonicalTokenUsageResult(requiredBody(result.body), {
      spaceId: spaceID,
      threadId: threadID,
    });
  }

  async listMemories(request: ListWorkbenchMemoriesRequest) {
    const { spaceID, threadID } = canonicalThreadScope(request);
    const { limit, offset } = productOffsetPagination(
      request.page,
      request.page_size,
    );
    const query = new URLSearchParams();
    appendMemoryQuery(query, request);
    query.set('limit', String(limit));
    query.set('offset', String(offset));
    const result = await fetchCanonicalJSON(
      `/api/workbench/threads/${threadID}/memories?${query.toString()}`,
      {
        fetch: this.fetcher,
        method: 'GET',
        spaceId: spaceID,
        signal: request.signal,
      },
    );
    return adaptCanonicalMemoryPage(requiredBody(result.body), {
      spaceId: spaceID,
      threadId: threadID,
    });
  }

  async updateMemory(request: UpdateWorkbenchMemoryRequest) {
    const { spaceID, threadID } = canonicalThreadScope(request);
    const memoryID = assertCanonicalRequestResourceID(
      request.memory_id,
      'memory_id',
    );
    const body = canonicalMemoryMutation(request);
    if (body.scope === undefined) {
      throw invalidCanonicalRequest(
        'missing_memory_scope',
        'scope is required for Memory update',
      );
    }
    const result = await fetchCanonicalJSON(
      `/api/workbench/threads/${threadID}/memories/${memoryID}`,
      {
        fetch: this.fetcher,
        method: 'PUT',
        spaceId: spaceID,
        json: body,
        signal: request.signal,
      },
    );
    return adaptCanonicalMemoryUpdate(
      requiredBody(result.body),
      { spaceId: spaceID, threadId: threadID },
      memoryID,
    );
  }

  async deleteMemory(request: WorkbenchMemoryRequest): Promise<void> {
    const { spaceID, threadID } = canonicalThreadScope(request);
    const memoryID = assertCanonicalRequestResourceID(
      request.memory_id,
      'memory_id',
    );
    const result = await fetchCanonicalJSON(
      `/api/workbench/threads/${threadID}/memories/${memoryID}`,
      {
        fetch: this.fetcher,
        method: 'DELETE',
        spaceId: spaceID,
        signal: request.signal,
      },
    );
    assertCanonicalEmptySuccess(result.body, 'Memory delete');
  }

  async restoreMemory(request: WorkbenchMemoryRequest) {
    const { spaceID, threadID } = canonicalThreadScope(request);
    const memoryID = assertCanonicalRequestResourceID(
      request.memory_id,
      'memory_id',
    );
    const result = await fetchCanonicalJSON(
      `/api/workbench/threads/${threadID}/memories/${memoryID}/restore`,
      {
        fetch: this.fetcher,
        method: 'POST',
        spaceId: spaceID,
        signal: request.signal,
      },
    );
    return adaptCanonicalMemoryRestore(
      requiredBody(result.body),
      { spaceId: spaceID, threadId: threadID },
      memoryID,
    );
  }

  async clearMemories(request: ClearWorkbenchMemoriesRequest) {
    const { spaceID, threadID } = canonicalThreadScope(request);
    const runID =
      request.run_id === undefined
        ? undefined
        : assertCanonicalRequestResourceID(request.run_id, 'run_id');
    const scopes = request.scopes?.map((scope, index) =>
      asNonEmptyRequestString(scope, `scopes[${index}]`),
    );
    const result = await fetchCanonicalJSON(
      `/api/workbench/threads/${threadID}/memories/clear`,
      {
        fetch: this.fetcher,
        method: 'POST',
        spaceId: spaceID,
        json: {
          ...(runID === undefined ? {} : { run_id: runID }),
          ...(scopes === undefined ? {} : { scopes }),
        },
        signal: request.signal,
      },
    );
    return adaptCanonicalMemoryClear(requiredBody(result.body));
  }

  async exportMemories(request: ExportWorkbenchMemoriesRequest) {
    const { spaceID, threadID } = canonicalThreadScope(request);
    if (request.page !== undefined && request.page !== 1) {
      throw invalidCanonicalRequest(
        'invalid_pagination',
        'Memory export supports only page 1',
      );
    }
    const query = new URLSearchParams();
    appendMemoryQuery(query, request);
    const limit = optionalPositiveInteger(request.page_size, 'page_size');
    if (limit !== undefined) {
      query.set('limit', String(limit));
    }
    const suffix = query.size === 0 ? '' : `?${query.toString()}`;
    const result = await fetchCanonicalJSON(
      `/api/workbench/threads/${threadID}/memories/export${suffix}`,
      {
        fetch: this.fetcher,
        method: 'GET',
        spaceId: spaceID,
        signal: request.signal,
      },
    );
    return adaptCanonicalMemoryExport(requiredBody(result.body), {
      spaceId: spaceID,
      threadId: threadID,
    });
  }

  async importMemories(request: ImportWorkbenchMemoriesRequest) {
    const { spaceID, threadID } = canonicalThreadScope(request);
    if (!Array.isArray(request.memories) || request.memories.length === 0) {
      throw invalidCanonicalRequest(
        'empty_memory_import',
        'memories must contain at least one item',
      );
    }
    const memories = request.memories.map(canonicalMemoryMutation);
    const result = await fetchCanonicalJSON(
      `/api/workbench/threads/${threadID}/memories/import`,
      {
        fetch: this.fetcher,
        method: 'POST',
        spaceId: spaceID,
        json: { memories },
        signal: request.signal,
      },
    );
    return adaptCanonicalMemoryImport(requiredBody(result.body), {
      spaceId: spaceID,
      threadId: threadID,
    });
  }

  async listMemoryAuditEvents(request: ListWorkbenchMemoryAuditEventsRequest) {
    const { spaceID, threadID } = canonicalThreadScope(request);
    const { limit, offset } = productOffsetPagination(
      request.page,
      request.page_size,
    );
    const query = new URLSearchParams();
    setOptionalResourceID(query, 'memory_id', request.memory_id);
    query.set('limit', String(limit));
    query.set('offset', String(offset));
    const result = await fetchCanonicalJSON(
      `/api/workbench/threads/${threadID}/memories/audit_events?${query.toString()}`,
      {
        fetch: this.fetcher,
        method: 'GET',
        spaceId: spaceID,
        signal: request.signal,
      },
    );
    return adaptCanonicalMemoryAuditPage(requiredBody(result.body), {
      spaceId: spaceID,
      threadId: threadID,
    });
  }

  async listGuardrailAuditEvents(
    request: ListWorkbenchGuardrailAuditEventsRequest,
  ) {
    const { spaceID, threadID } = canonicalThreadScope(request);
    const { limit, offset } = productOffsetPagination(
      request.page,
      request.page_size,
    );
    const query = new URLSearchParams();
    setOptionalResourceID(query, 'run_id', request.run_id);
    query.set('limit', String(limit));
    query.set('offset', String(offset));
    const result = await fetchCanonicalJSON(
      `/api/workbench/threads/${threadID}/guardrail_audit_events?${query.toString()}`,
      {
        fetch: this.fetcher,
        method: 'GET',
        spaceId: spaceID,
        signal: request.signal,
      },
    );
    return adaptCanonicalGuardrailAuditPage(requiredBody(result.body), {
      spaceId: spaceID,
      threadId: threadID,
    });
  }

  async exportGuardrailAuditEvents(
    request: ExportWorkbenchGuardrailAuditEventsRequest,
  ) {
    const { spaceID, threadID } = canonicalThreadScope(request);
    const { limit, offset } = productOffsetPagination(
      request.page,
      request.page_size,
    );
    const query = new URLSearchParams();
    setOptionalResourceID(query, 'run_id', request.run_id);
    query.set('limit', String(limit));
    query.set('offset', String(offset));
    const result = await fetchCanonicalJSON(
      `/api/workbench/threads/${threadID}/guardrail_audit_events/export?${query.toString()}`,
      {
        fetch: this.fetcher,
        method: 'GET',
        spaceId: spaceID,
        signal: request.signal,
      },
    );
    return adaptCanonicalGuardrailAuditExport(requiredBody(result.body), {
      spaceId: spaceID,
      threadId: threadID,
    });
  }

  async listMCPRuntimeAuditEvents(
    request: ListWorkbenchMCPRuntimeAuditEventsRequest,
  ) {
    const { spaceID, threadID } = canonicalThreadScope(request);
    const { limit, offset } = productOffsetPagination(
      request.page,
      request.page_size,
    );
    const query = new URLSearchParams();
    setOptionalResourceID(query, 'run_id', request.run_id);
    query.set('limit', String(limit));
    query.set('offset', String(offset));
    const result = await fetchCanonicalJSON(
      `/api/workbench/threads/${threadID}/mcp_runtime_audit_events?${query.toString()}`,
      {
        fetch: this.fetcher,
        method: 'GET',
        spaceId: spaceID,
        signal: request.signal,
      },
    );
    return adaptCanonicalMCPRuntimeAuditPage(requiredBody(result.body), {
      spaceId: spaceID,
      threadId: threadID,
    });
  }
}

export { CanonicalThreadCoreClient as CanonicalThreadClient };
