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

/* eslint-disable max-lines -- Task 3 intentionally exposes only the implemented core surface. */

import type {
  CancelWorkbenchRunRequest,
  CreateWorkbenchRunRequest,
  CreateWorkbenchThreadRequest,
  GetWorkbenchRunRequest,
  GetWorkbenchThreadRequest,
  ListWorkbenchMessagesRequest,
  ListWorkbenchRunEventsRequest,
  ListWorkbenchRunsRequest,
  ResumeWorkbenchRunRequest,
  SearchWorkbenchThreadsRequest,
  WorkbenchThreadClient,
} from './workbench-thread-client';
import {
  fetchCanonicalJSON,
  invalidCanonicalRequest,
  invalidCanonicalResponse,
  type CanonicalFetch,
} from './canonical-fetch';
import {
  adaptCanonicalMessagePage,
  adaptCanonicalRun,
  adaptCanonicalRunCreation,
  adaptCanonicalRunEventPage,
  adaptCanonicalRunList,
  adaptCanonicalThread,
  adaptCanonicalThreadCreation,
  adaptCanonicalThreadList,
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

export interface CanonicalThreadCoreClientOptions {
  fetch?: CanonicalFetch;
}

const canonicalPublicAssistantID = 'agent';
const defaultPageSize = 20;
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

const offsetPagination = (page?: number, pageSize?: number) => {
  const resolvedPage = asPageValue(page, 1, 'page');
  const limit = asPageValue(pageSize, defaultPageSize, 'page_size');
  const offset = (resolvedPage - 1) * limit;
  if (!Number.isSafeInteger(offset) || offset > maxCanonicalPageValue) {
    throw invalidCanonicalRequest(
      'invalid_pagination',
      'page and page_size produce an unsupported offset',
    );
  }
  return { limit, offset };
};

const cursorLimit = (value?: number): number =>
  asPageValue(value, defaultPageSize, 'limit');

const optionalTrimmed = (value: string | undefined): string | undefined => {
  const trimmed = value?.trim();
  return trimmed ? trimmed : undefined;
};

const requestAssistantID = (value: string | undefined): string =>
  optionalTrimmed(value) ?? canonicalPublicAssistantID;

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

export class CanonicalThreadCoreClient implements CanonicalWorkbenchCoreClient {
  readonly contract = 'canonical_v1' as const;

  private readonly fetcher: CanonicalFetch | undefined;

  constructor(options: CanonicalThreadCoreClientOptions = {}) {
    this.fetcher = options.fetch;
  }

  async searchThreads(request: SearchWorkbenchThreadsRequest) {
    const spaceID = assertCanonicalRequestResourceID(
      request.space_id,
      'space_id',
    );
    const { limit, offset } = offsetPagination(request.page, request.page_size);
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
    const config = parseCanonicalWriteJSONObject(request.config, 'config');
    const context = parseCanonicalWriteJSONObject(request.context, 'context');
    const metadata = parseCanonicalWriteJSONObject(
      request.metadata,
      'metadata',
    );
    parseCanonicalWriteJSON(request.command, 'command');
    unsupportedThreadCreateOptions(request);

    const message = asNonEmptyRequestString(request.message, 'message');
    const title = optionalTrimmed(request.title);
    const initialRun = {
      assistant_id: requestAssistantID(request.assistant_id),
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
    const { limit, offset } = offsetPagination(request.page, request.page_size);
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
    const input = parseRequiredCanonicalWriteJSONObject(request.input, 'input');
    const command = parseCanonicalWriteJSONObject(request.command, 'command');
    const config = parseCanonicalWriteJSONObject(request.config, 'config');
    const context = parseCanonicalWriteJSONObject(request.context, 'context');
    const metadata = parseCanonicalWriteJSONObject(
      request.metadata,
      'metadata',
    );
    parseCanonicalWriteJSON(request.message_metadata, 'message_metadata');
    const messageContent = asNonEmptyRequestString(
      request.message_content,
      'message_content',
    );
    const streamMode = canonicalStreamMode(request.stream_mode);
    const body = {
      assistant_id: requestAssistantID(request.assistant_id),
      input: {
        messages: [{ role: 'user', content: messageContent }],
        uploaded_files: canonicalUploadedFiles(input),
      },
      ...(command === undefined ? {} : { command }),
      ...(config === undefined ? {} : { config }),
      ...(context === undefined ? {} : { context }),
      ...(metadata === undefined ? {} : { metadata }),
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
}
