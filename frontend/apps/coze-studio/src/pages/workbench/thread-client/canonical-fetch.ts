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

export type WorkbenchClientOutcome = 'rejected' | 'failed' | 'unknown';

interface WorkbenchClientErrorOptions {
  message: string;
  status?: number;
  code: string;
  traceId?: string;
  retryable: boolean;
  outcome: WorkbenchClientOutcome;
}

export class WorkbenchClientError extends Error {
  readonly status: number | undefined;
  readonly code: string;
  readonly traceId: string | undefined;
  readonly retryable: boolean;
  readonly outcome: WorkbenchClientOutcome;

  constructor(options: WorkbenchClientErrorOptions) {
    super(options.message);
    this.name = 'WorkbenchClientError';
    this.status = options.status;
    this.code = options.code;
    this.traceId = options.traceId;
    this.retryable = options.retryable;
    this.outcome = options.outcome;
  }
}

export type CanonicalFetch = (
  input: RequestInfo | URL,
  init?: RequestInit,
) => Promise<Response>;

export interface CanonicalPagination {
  total?: number;
  next?: string;
}

export interface CanonicalJSONResult<T = unknown> {
  body: T | undefined;
  pagination: CanonicalPagination;
}

export interface CanonicalJSONRequest {
  fetch?: CanonicalFetch;
  method: string;
  spaceId: string;
  json?: unknown;
  body?: BodyInit;
  idempotencyKey?: string;
  signal?: AbortSignal;
}

export interface CanonicalBlobResult {
  blob: Blob;
  contentType: string | null;
  contentDisposition: string | null;
}

interface CanonicalErrorBody {
  detail: string;
  code: string;
  retryable: boolean;
  trace_id: string;
}

const canonicalErrorKeys = ['code', 'detail', 'retryable', 'trace_id'] as const;
const httpClientErrorStart = 400;
const httpServerErrorStart = 500;
const httpServerErrorEnd = 600;
const httpNoContent = 204;

const isRecord = (value: unknown): value is Record<string, unknown> =>
  typeof value === 'object' && value !== null && !Array.isArray(value);

const assertJSONValue = (
  value: unknown,
  label: string,
  seen = new WeakSet<object>(),
): void => {
  if (
    value === null ||
    typeof value === 'string' ||
    typeof value === 'boolean'
  ) {
    return;
  }
  if (typeof value === 'number') {
    if (
      !Number.isFinite(value) ||
      (Number.isInteger(value) && !Number.isSafeInteger(value))
    ) {
      throw invalidCanonicalRequest(
        'invalid_request_json',
        `${label} contains an unsafe number`,
      );
    }
    return;
  }
  if (typeof value !== 'object') {
    throw invalidCanonicalRequest(
      'invalid_request_json',
      `${label} contains a non-JSON value`,
    );
  }
  if (seen.has(value)) {
    throw invalidCanonicalRequest(
      'invalid_request_json',
      `${label} contains a cycle`,
    );
  }
  seen.add(value);
  if (Array.isArray(value)) {
    value.forEach((item, index) =>
      assertJSONValue(item, `${label}[${index}]`, seen),
    );
  } else {
    Object.entries(value).forEach(([key, item]) =>
      assertJSONValue(item, `${label}.${key}`, seen),
    );
  }
  seen.delete(value);
};

const serializeJSON = (value: unknown): string => {
  assertJSONValue(value, 'request body');
  try {
    const encoded = JSON.stringify(value);
    if (typeof encoded !== 'string') {
      throw new TypeError('request body is not JSON serializable');
    }
    return encoded;
  } catch (error) {
    if (error instanceof WorkbenchClientError) {
      throw error;
    }
    throw invalidCanonicalRequest(
      'invalid_request_json',
      'Request body is not valid JSON',
    );
  }
};

const responseOutcome = (status: number): WorkbenchClientOutcome => {
  if (status >= httpClientErrorStart && status < httpServerErrorStart) {
    return 'rejected';
  }
  if (status >= httpServerErrorStart && status < httpServerErrorEnd) {
    return 'failed';
  }
  return 'unknown';
};

const isAbortFailure = (error: unknown): boolean => {
  if (!isRecord(error)) {
    return false;
  }
  return error.name === 'AbortError' || error.code === 'ABORT_ERR';
};

const parseJSONResponse = async (
  response: Response,
  kind: 'success' | 'error',
): Promise<unknown> => {
  let raw: string;
  try {
    raw = await response.text();
  } catch (error) {
    if (isAbortFailure(error)) {
      throw error;
    }
    throw invalidCanonicalResponse(
      `Canonical ${kind} response could not be read`,
      response.status,
    );
  }
  if (raw.trim() === '') {
    throw invalidCanonicalResponse(
      `Canonical ${kind} response body is empty`,
      response.status,
    );
  }
  try {
    const parsed = JSON.parse(raw) as unknown;
    assertResponseJSONValue(parsed, {
      status: response.status,
      label: 'response body',
    });
    return parsed;
  } catch (error) {
    if (error instanceof WorkbenchClientError) {
      throw error;
    }
    throw invalidCanonicalResponse(
      `Canonical ${kind} response is malformed JSON`,
      response.status,
    );
  }
};

interface ResponseJSONValidation {
  status: number;
  label: string;
  seen?: WeakSet<object>;
}

const assertResponseJSONValue = (
  value: unknown,
  validation: ResponseJSONValidation,
): void => {
  const { status, label } = validation;
  const seen = validation.seen ?? new WeakSet<object>();
  if (
    value === null ||
    typeof value === 'string' ||
    typeof value === 'boolean'
  ) {
    return;
  }
  if (typeof value === 'number') {
    if (
      !Number.isFinite(value) ||
      (Number.isInteger(value) && !Number.isSafeInteger(value))
    ) {
      throw invalidCanonicalResponse(
        `${label} contains an unsafe number`,
        status,
      );
    }
    return;
  }
  if (typeof value !== 'object') {
    throw invalidCanonicalResponse(
      `${label} contains a non-JSON value`,
      status,
    );
  }
  if (seen.has(value)) {
    throw invalidCanonicalResponse(`${label} contains a cycle`, status);
  }
  seen.add(value);
  if (Array.isArray(value)) {
    value.forEach((item, index) =>
      assertResponseJSONValue(item, {
        status,
        label: `${label}[${index}]`,
        seen,
      }),
    );
  } else {
    Object.entries(value).forEach(([key, item]) =>
      assertResponseJSONValue(item, {
        status,
        label: `${label}.${key}`,
        seen,
      }),
    );
  }
  seen.delete(value);
};

const parseCanonicalError = (
  value: unknown,
  status: number,
): CanonicalErrorBody => {
  if (!isRecord(value)) {
    throw invalidCanonicalResponse(
      'Canonical error response must be an object',
      status,
    );
  }
  const keys = Object.keys(value).sort();
  if (
    keys.length !== canonicalErrorKeys.length ||
    canonicalErrorKeys.some((key, index) => keys[index] !== key)
  ) {
    throw invalidCanonicalResponse(
      'Canonical error response has an unknown shape',
      status,
    );
  }
  const { detail, code, retryable, trace_id: traceID } = value;
  if (
    typeof detail !== 'string' ||
    detail.trim() === '' ||
    typeof code !== 'string' ||
    code.trim() === '' ||
    typeof retryable !== 'boolean' ||
    typeof traceID !== 'string'
  ) {
    throw invalidCanonicalResponse(
      'Canonical error response has invalid fields',
      status,
    );
  }
  return { detail, code, retryable, trace_id: traceID };
};

export const canonicalErrorFromResponse = async (
  response: Response,
): Promise<WorkbenchClientError> => {
  const parsed = await parseJSONResponse(response, 'error');
  const canonical = parseCanonicalError(parsed, response.status);
  return new WorkbenchClientError({
    message: canonical.detail,
    status: response.status,
    code: canonical.code,
    traceId: canonical.trace_id,
    retryable: canonical.retryable,
    outcome: responseOutcome(response.status),
  });
};

const parsePaginationHeader = (
  response: Response,
  name: 'X-Pagination-Total' | 'X-Pagination-Next',
): { number: number; raw: string } | undefined => {
  const raw = response.headers.get(name);
  if (raw === null) {
    return undefined;
  }
  if (!/^(0|[1-9]\d*)$/.test(raw)) {
    throw invalidCanonicalResponse(
      `${name} must be a non-negative decimal integer`,
      response.status,
    );
  }
  const number = Number(raw);
  if (!Number.isSafeInteger(number)) {
    throw invalidCanonicalResponse(
      `${name} exceeds the safe integer range`,
      response.status,
    );
  }
  return { number, raw };
};

const canonicalPagination = (response: Response): CanonicalPagination => {
  const total = parsePaginationHeader(response, 'X-Pagination-Total');
  const next = parsePaginationHeader(response, 'X-Pagination-Next');
  return {
    ...(total ? { total: total.number } : {}),
    ...(next ? { next: next.raw } : {}),
  };
};

export const invalidCanonicalRequest = (
  code: string,
  message: string,
): WorkbenchClientError =>
  new WorkbenchClientError({
    message,
    code,
    retryable: false,
    outcome: 'unknown',
  });

export const invalidCanonicalResponse = (
  message: string,
  status = 200,
): WorkbenchClientError =>
  new WorkbenchClientError({
    message,
    status,
    code: 'invalid_response',
    retryable: false,
    outcome: 'unknown',
  });

const fetchCanonicalResponse = async (
  url: string,
  request: CanonicalJSONRequest,
): Promise<Response> => {
  const headers: Record<string, string> = {
    'X-Coze-Space-ID': request.spaceId,
    'x-requested-with': 'XMLHttpRequest',
  };
  if (request.json !== undefined && request.body !== undefined) {
    throw invalidCanonicalRequest(
      'invalid_request_body',
      'Canonical request cannot contain both JSON and raw body data',
    );
  }
  let { body } = request;
  if (request.json !== undefined) {
    headers['content-type'] = 'application/json';
    body = serializeJSON(request.json);
  }
  if (request.idempotencyKey) {
    headers['Idempotency-Key'] = request.idempotencyKey;
  }

  const fetcher = request.fetch ?? globalThis.fetch;
  if (typeof fetcher !== 'function') {
    throw new WorkbenchClientError({
      message: 'Canonical transport is unavailable',
      code: 'network_error',
      retryable: false,
      outcome: 'unknown',
    });
  }

  let response: Response;
  try {
    response = await fetcher(url, {
      method: request.method,
      credentials: 'same-origin',
      headers,
      ...(body === undefined ? {} : { body }),
      signal: request.signal,
    });
  } catch (error) {
    if (isAbortFailure(error)) {
      throw error;
    }
    throw new WorkbenchClientError({
      message: 'Canonical request failed before receiving an HTTP response',
      code: 'network_error',
      retryable: false,
      outcome: 'unknown',
    });
  }

  if (!response.ok) {
    throw await canonicalErrorFromResponse(response);
  }

  return response;
};

export const fetchCanonicalJSON = async <T = unknown>(
  url: string,
  request: CanonicalJSONRequest,
): Promise<CanonicalJSONResult<T>> => {
  const response = await fetchCanonicalResponse(url, request);

  const pagination = canonicalPagination(response);
  if (response.status === httpNoContent) {
    return { body: undefined, pagination };
  }
  const parsed = await parseJSONResponse(response, 'success');
  if (!isRecord(parsed) && !Array.isArray(parsed)) {
    throw invalidCanonicalResponse(
      'Canonical success response has an unknown shape',
      response.status,
    );
  }
  return { body: parsed as T, pagination };
};

export const fetchCanonicalBlob = async (
  url: string,
  request: CanonicalJSONRequest,
): Promise<CanonicalBlobResult> => {
  const response = await fetchCanonicalResponse(url, request);
  if (response.status === httpNoContent) {
    throw invalidCanonicalResponse(
      'Canonical Blob response body is empty',
      response.status,
    );
  }
  let blob: Blob;
  try {
    blob = await response.blob();
  } catch (error) {
    if (isAbortFailure(error)) {
      throw error;
    }
    throw invalidCanonicalResponse(
      'Canonical Blob response could not be read',
      response.status,
    );
  }
  return {
    blob,
    contentType: response.headers.get('content-type'),
    contentDisposition: response.headers.get('content-disposition'),
  };
};
