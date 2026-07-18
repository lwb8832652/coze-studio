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

/* eslint-disable max-lines, no-control-regex -- Cohesive orchestrator; control-character rejection is intentional. */

import { normalizeAppDevSnapshotID } from './utils/snapshot-id';
import type {
  AppDevApiResponse,
  AppDevBuildInfo,
  AppDevChatAttachment,
  AppDevChatMessage,
  AppDevChatSession,
  AppDevChatStatus,
  AppDevCreateProjectParams,
  AppDevDataSource,
  AppDevFileContent,
  AppDevFileNode,
  AppDevModel,
  AppDevPageResult,
  AppDevProject,
  AppDevProjectIdentity,
  AppDevRuntimeInfo,
  AppDevRuntimeLog,
  AppDevRuntimeStatus,
  AppDevSnapshot,
  AppDevBuildSafeErrorCode,
} from './types';
import { APP_DEV_BUILD_SAFE_ERROR_CODES } from './types';

const jsonHeaders = {
  'content-type': 'application/json',
};

const buildProjectBasePath = ({ spaceId, projectId }: AppDevProjectIdentity) =>
  `/api/app-dev/spaces/${encodeURIComponent(
    spaceId,
  )}/projects/${encodeURIComponent(projectId)}`;

const buildQuery = (params: Record<string, string | undefined>) => {
  const searchParams = new URLSearchParams();

  Object.entries(params).forEach(([key, value]) => {
    if (value?.trim()) {
      searchParams.set(key, value.trim());
    }
  });

  const query = searchParams.toString();
  return query ? `?${query}` : '';
};

export type AppDevSafeErrorCode =
  | 'invalid_input'
  | 'unauthorized'
  | 'forbidden'
  | 'not_found'
  | 'conflict'
  | 'unavailable'
  | 'invalid_response'
  | 'release_stale'
  | 'workspace_disabled';

const appDevSafeMessages: Record<AppDevSafeErrorCode, string> = {
  invalid_input: '请求参数无效，请刷新后重试',
  unauthorized: '登录状态已失效，请重新登录后重试',
  forbidden: '当前账号无权执行此操作',
  not_found: '项目或运行环境不存在',
  conflict: '操作与当前状态冲突，请刷新后重试',
  unavailable: '运行服务暂不可用，请稍后重试',
  invalid_response: '运行服务返回格式异常，请稍后重试',
  release_stale: '发布产物已过期，请重新构建',
  workspace_disabled:
    '当前工作空间未开启网页应用开发，请在“工作空间”设置中开启后重试',
};

export class AppDevSafeError extends Error {
  readonly code: AppDevSafeErrorCode;

  constructor(code: AppDevSafeErrorCode) {
    super(appDevSafeMessages[code]);
    this.name = 'AppDevSafeError';
    this.code = code;
  }
}

export const isAppDevDeterministicError = (error: unknown) =>
  error instanceof AppDevSafeError &&
  [
    'invalid_input',
    'unauthorized',
    'forbidden',
    'not_found',
    'conflict',
  ].includes(error.code);

export const isAppDevReleaseStaleError = (error: unknown) =>
  error instanceof AppDevSafeError && error.code === 'release_stale';

export const normalizeAppDevError = (error: unknown) =>
  error instanceof AppDevSafeError ? error.message : '请求失败，请稍后重试';

const safeErrorForStatus = (status: number) => {
  if (status === 400) {
    return new AppDevSafeError('invalid_input');
  }
  if (status === 401) {
    return new AppDevSafeError('unauthorized');
  }
  if (status === 403) {
    return new AppDevSafeError('forbidden');
  }
  if (status === 404) {
    return new AppDevSafeError('not_found');
  }
  if (status === 409) {
    return new AppDevSafeError('conflict');
  }
  return new AppDevSafeError('unavailable');
};

const readJson = async <T>(
  response: Response,
): Promise<AppDevApiResponse<T>> => {
  if (response.headers.get('content-type')?.includes('application/json')) {
    const payload: AppDevApiResponse<T> = await response.json();
    return payload;
  }

  const payload: AppDevApiResponse<T> = {};
  return payload;
};

const requestAppDev = async <T>(
  path: string,
  parse: (data: unknown) => T,
  init?: RequestInit,
): Promise<T> => {
  let response: Response;
  try {
    response = await fetch(path, init);
  } catch (error) {
    if (init?.signal?.aborted) {
      throw error;
    }
    throw new AppDevSafeError('unavailable');
  }
  if (!response.headers.get('content-type')?.includes('application/json')) {
    if (!response.ok) {
      throw safeErrorForStatus(response.status);
    }
    throw new AppDevSafeError('invalid_response');
  }
  let payload: unknown;
  try {
    payload = await response.json();
  } catch {
    throw new AppDevSafeError('invalid_response');
  }
  if (!isRecord(payload)) {
    throw new AppDevSafeError('invalid_response');
  }
  if (
    payload.code !== undefined &&
    (typeof payload.code !== 'number' || !Number.isFinite(payload.code))
  ) {
    throw new AppDevSafeError('invalid_response');
  }
  if (!response.ok || (payload.code !== undefined && payload.code !== 0)) {
    const rawMessage = `${payload.message || payload.msg || ''}`.toLowerCase();
    if (rawMessage.includes('workspace does not allow development')) {
      throw new AppDevSafeError('workspace_disabled');
    }
    throw safeErrorForStatus(response.status);
  }
  if (!Object.prototype.hasOwnProperty.call(payload, 'data')) {
    throw new AppDevSafeError('invalid_response');
  }
  try {
    return parse(payload.data);
  } catch (error) {
    if (error instanceof AppDevSafeError) {
      throw error;
    }
    throw new AppDevSafeError('invalid_response');
  }
};

interface ProviderRuntimeWire {
  generation: number;
  state: string;
  can_start: boolean;
  recovering: boolean;
  stopping: boolean;
  preview_url?: string;
  safe_message?: string;
}

interface ProviderBuildWire {
  generation: number;
  state: string;
  release_available: boolean;
  size: number;
  updated_at?: string;
  stale: boolean;
  safe_error_code?: unknown;
}

const isRecord = (value: unknown): value is Record<string, unknown> =>
  Boolean(value) && typeof value === 'object' && !Array.isArray(value);

const safeProviderMessage = (value: unknown) =>
  typeof value === 'string' &&
  value.length > 0 &&
  value.length <= 256 &&
  !/[\u0000-\u0008\u000b\u000c\u000e-\u001f\u007f]/u.test(value)
    ? value
    : undefined;

const requestProvider = async <T>(
  path: string,
  parse: (payload: unknown) => T,
  init?: RequestInit,
): Promise<T> => {
  let response: Response;
  try {
    response = await fetch(path, { credentials: 'include', ...init });
  } catch (error) {
    if (init?.signal?.aborted) {
      throw error;
    }
    throw new AppDevSafeError('unavailable');
  }
  let payload: unknown;
  if (response.headers.get('content-type')?.includes('application/json')) {
    try {
      payload = await response.json();
    } catch {
      throw new AppDevSafeError('invalid_response');
    }
  }
  if (!response.ok) {
    throw safeErrorForStatus(response.status);
  }
  try {
    return parse(payload);
  } catch (error) {
    if (error instanceof AppDevSafeError) {
      throw error;
    }
    throw new AppDevSafeError('invalid_response');
  }
};

const requireProviderObject = (payload: unknown) => {
  if (!isRecord(payload)) {
    throw new Error('invalid provider payload');
  }
  return payload;
};

const requireProviderNumber = (value: unknown) => {
  if (!Number.isSafeInteger(value) || Number(value) < 0) {
    throw new Error('invalid provider number');
  }
  return Number(value);
};

const requireProviderBoolean = (value: unknown) => {
  if (typeof value !== 'boolean') {
    throw new Error('invalid provider boolean');
  }
  return value;
};

const requireString = (value: unknown) => {
  if (typeof value !== 'string') {
    throw new AppDevSafeError('invalid_response');
  }
  return value;
};

const requireBoolean = (value: unknown) => {
  if (typeof value !== 'boolean') {
    throw new AppDevSafeError('invalid_response');
  }
  return value;
};

const requireSafeNumber = (value: unknown) => {
  if (!Number.isSafeInteger(value) || Number(value) < 0) {
    throw new AppDevSafeError('invalid_response');
  }
  return Number(value);
};

const optionalString = (value: unknown) =>
  value === undefined ? undefined : requireString(value);

const requireDataObject = (value: unknown) => {
  if (!isRecord(value)) {
    throw new AppDevSafeError('invalid_response');
  }
  return value;
};

const projectStatuses = new Set<AppDevProject['status']>([
  'creating',
  'ready',
  'archived',
  'error',
]);
const runtimeStatuses = new Set<NonNullable<AppDevProject['runtimeStatus']>>([
  'stopped',
  'starting',
  'running',
  'recovering',
  'stopping',
  'cleanup_pending',
  'error',
]);

const parseAppDevProject = (value: unknown): AppDevProject => {
  const data = requireDataObject(value);
  const status = requireString(data.status);
  if (!projectStatuses.has(status as AppDevProject['status'])) {
    throw new AppDevSafeError('invalid_response');
  }
  const runtimeStatus = optionalString(data.runtimeStatus);
  if (
    runtimeStatus &&
    !runtimeStatuses.has(
      runtimeStatus as NonNullable<AppDevProject['runtimeStatus']>,
    )
  ) {
    throw new AppDevSafeError('invalid_response');
  }
  const lastBuildStatus = optionalString(data.lastBuildStatus);
  if (
    lastBuildStatus &&
    !['building', 'success', 'error'].includes(lastBuildStatus)
  ) {
    throw new AppDevSafeError('invalid_response');
  }
  return {
    id: requireString(data.id),
    name: requireString(data.name),
    status: status as AppDevProject['status'],
    description: optionalString(data.description),
    prompt: optionalString(data.prompt),
    runtimeStatus: runtimeStatus as AppDevProject['runtimeStatus'],
    previewUrl: optionalString(data.previewUrl),
    lastBuildStatus: lastBuildStatus as AppDevProject['lastBuildStatus'],
    lastBuildType: optionalString(data.lastBuildType),
    lastBuildMessage: optionalString(data.lastBuildMessage),
    lastBuildAt: optionalString(data.lastBuildAt),
    sourceUpdatedAt: optionalString(data.sourceUpdatedAt),
    creatorName: optionalString(data.creatorName),
    updatedAt: optionalString(data.updatedAt),
    createdAt: optionalString(data.createdAt),
  };
};

const parseItems = <T>(value: unknown, parseItem: (item: unknown) => T) => {
  const data = requireDataObject(value);
  if (!Array.isArray(data.items)) {
    throw new AppDevSafeError('invalid_response');
  }
  return { items: data.items.map(parseItem) };
};

const parseProjectPage = (value: unknown): AppDevPageResult<AppDevProject> => {
  const data = requireDataObject(value);
  if (!Array.isArray(data.items)) {
    throw new AppDevSafeError('invalid_response');
  }
  return {
    items: data.items.map(parseAppDevProject),
    total: requireSafeNumber(data.total),
  };
};

const parseSuccess = (value: unknown) => {
  const data = requireDataObject(value);
  return { success: requireBoolean(data.success) };
};

const parseFileNode = (value: unknown): AppDevFileNode => {
  const data = requireDataObject(value);
  const type = requireString(data.type);
  if (type !== 'file' && type !== 'directory') {
    throw new AppDevSafeError('invalid_response');
  }
  if (data.children !== undefined && !Array.isArray(data.children)) {
    throw new AppDevSafeError('invalid_response');
  }
  return {
    id: requireString(data.id),
    path: requireString(data.path),
    name: requireString(data.name),
    type,
    size: data.size === undefined ? undefined : requireSafeNumber(data.size),
    children: data.children?.map(parseFileNode),
    updatedAt: optionalString(data.updatedAt),
  };
};

const parseFileContent = (value: unknown): AppDevFileContent => {
  const data = requireDataObject(value);
  return {
    path: requireString(data.path),
    content: requireString(data.content),
    version: requireString(data.version),
    language: data.language === undefined ? '' : requireString(data.language),
  };
};

const parseUploaded = (value: unknown) => {
  const data = requireDataObject(value);
  return { uploaded: requireSafeNumber(data.uploaded) };
};

const parseSnapshot = (value: unknown): AppDevSnapshot => {
  const data = requireDataObject(value);
  const id = normalizeAppDevSnapshotID(data.id);
  if (!id) {
    throw new AppDevSafeError('invalid_response');
  }
  return {
    id,
    label: requireString(data.label),
    createdAt: requireString(data.createdAt),
  };
};

const parseRecordItems = (value: unknown) => {
  const data = requireDataObject(value);
  if (!Array.isArray(data.items)) {
    throw new AppDevSafeError('invalid_response');
  }
  return {
    items: data.items.map(item => requireDataObject(item)),
    total: data.total === undefined ? undefined : requireSafeNumber(data.total),
  };
};

const parseChatSession = (value: unknown): AppDevChatSession => {
  const data = requireDataObject(value);
  return {
    sessionId: requireString(data.sessionId),
    requestId: requireString(data.requestId),
    running: requireBoolean(data.running),
  };
};

const parseChatStatus = (value: unknown): AppDevChatStatus => {
  const data = requireDataObject(value);
  return {
    running: requireBoolean(data.running),
    sessionId: optionalString(data.sessionId),
    requestId: optionalString(data.requestId),
  };
};

const parseChatMessage = (value: unknown): AppDevChatMessage => {
  const data = requireDataObject(value);
  const type = requireString(data.type);
  if (
    ![
      'user',
      'assistant',
      'thinking',
      'tool_call',
      'tool_call_update',
      'error',
      'section',
    ].includes(type)
  ) {
    throw new AppDevSafeError('invalid_response');
  }
  return {
    id: requireString(data.id),
    type: type as AppDevChatMessage['type'],
    content: requireString(data.content),
    title: optionalString(data.title),
    isStreaming:
      data.isStreaming === undefined
        ? undefined
        : requireBoolean(data.isStreaming),
    createdAt: optionalString(data.createdAt),
  };
};

const normalizeRuntimeState = (
  wire: ProviderRuntimeWire,
): AppDevRuntimeInfo['status'] => {
  if (wire.recovering) {
    return 'recovering';
  }
  if (wire.state === 'cleanup_pending') {
    return 'cleanup_pending';
  }
  if (wire.stopping) {
    return 'stopping';
  }
  if (['pending', 'submitting', 'accepted', 'starting'].includes(wire.state)) {
    return 'starting';
  }
  if (wire.state === 'running') {
    return 'running';
  }
  if (wire.state === 'stopped' || wire.state === 'cleanup_complete') {
    return 'stopped';
  }
  if (wire.state === 'succeeded' || wire.state === 'canceled') {
    return 'stopping';
  }
  if (wire.state === 'failed' || wire.state === 'timed_out') {
    return 'error';
  }
  throw new Error('invalid runtime state');
};

const parseProviderRuntime = (payload: unknown): AppDevRuntimeInfo => {
  const value = requireProviderObject(payload);
  if (
    typeof value.state !== 'string' ||
    (value.preview_url !== undefined && typeof value.preview_url !== 'string')
  ) {
    throw new Error('invalid runtime payload');
  }
  const wire: ProviderRuntimeWire = {
    generation: requireProviderNumber(value.generation),
    state: value.state,
    can_start: requireProviderBoolean(value.can_start),
    recovering: requireProviderBoolean(value.recovering),
    stopping: requireProviderBoolean(value.stopping),
    preview_url: value.preview_url,
    safe_message: undefined,
  };
  return {
    generation: wire.generation,
    status: normalizeRuntimeState(wire),
    canStart: wire.can_start,
    recovering: wire.recovering,
    stopping: wire.stopping,
    previewUrl: wire.preview_url,
    message: wire.safe_message,
  };
};

const providerBuildSafeMessages: Readonly<
  Record<AppDevBuildSafeErrorCode, string>
> = {
  build_failed: '构建失败，请稍后重试',
  source_invalid: '源码无法构建，请检查后重试',
  dependency_failed: '项目依赖构建失败，请检查依赖后重试',
  provider_unavailable: '构建服务暂不可用，请稍后重试',
  provider_capability_missing: '构建服务不支持此操作',
  provider_contract_violation: '构建服务返回异常，请稍后重试',
  build_timed_out: '构建超时，请重试',
  build_canceled: '构建已取消',
  artifact_invalid: '构建产物校验失败，请重新构建',
};

const providerBuildSafeCodes = new Set<unknown>(APP_DEV_BUILD_SAFE_ERROR_CODES);

const normalizeProviderBuildSafeCode = (
  value: unknown,
): AppDevBuildSafeErrorCode =>
  providerBuildSafeCodes.has(value)
    ? (value as AppDevBuildSafeErrorCode)
    : 'build_failed';

const parseProviderBuild = (payload: unknown): AppDevBuildInfo => {
  const value = requireProviderObject(payload);
  if (
    !['idle', 'building', 'ready', 'failed'].includes(String(value.state)) ||
    (value.updated_at !== undefined && typeof value.updated_at !== 'string')
  ) {
    throw new Error('invalid build payload');
  }
  const wire: ProviderBuildWire = {
    generation: requireProviderNumber(value.generation),
    state: String(value.state),
    release_available: requireProviderBoolean(value.release_available),
    size: requireProviderNumber(value.size),
    updated_at: value.updated_at as string | undefined,
    stale: requireProviderBoolean(value.stale),
    safe_error_code: value.safe_error_code,
  };
  const safeCode =
    wire.state === 'failed'
      ? normalizeProviderBuildSafeCode(wire.safe_error_code)
      : undefined;
  return {
    generation: wire.generation,
    state: wire.state as AppDevBuildInfo['state'],
    releaseAvailable: wire.release_available,
    size: wire.size,
    updatedAt: wire.updated_at,
    stale: wire.stale,
    safeErrorCode: safeCode,
    safeMessage: safeCode
      ? providerBuildSafeMessages[safeCode]
      : wire.state === 'failed'
        ? '构建失败，请稍后重试'
        : undefined,
  };
};

const validOperationID = (value: string) =>
  value.length >= 8 &&
  value.length <= 128 &&
  value === value.trim() &&
  /^[A-Za-z0-9._:-]+$/u.test(value);

const providerOperationHeaders = (operationId: string) => {
  if (!validOperationID(operationId)) {
    throw new Error('操作标识无效，请重新发起操作');
  }
  return { ...jsonHeaders, 'Idempotency-Key': operationId };
};

export const listAppDevProjects = async ({
  spaceId,
  keyword,
}: {
  spaceId: string;
  keyword?: string;
}) =>
  requestAppDev<AppDevPageResult<AppDevProject>>(
    `/api/app-dev/spaces/${encodeURIComponent(
      spaceId,
    )}/projects${buildQuery({ keyword })}`,
    parseProjectPage,
  );

export const createAppDevProject = async ({
  spaceId,
  name,
  prompt,
}: AppDevCreateProjectParams) =>
  requestAppDev<AppDevProject>(
    `/api/app-dev/spaces/${encodeURIComponent(spaceId)}/projects`,
    parseAppDevProject,
    {
      method: 'POST',
      headers: jsonHeaders,
      body: JSON.stringify({ name, prompt }),
    },
  );

export const importAppDevProject = async ({
  spaceId,
  file,
  name,
}: {
  spaceId: string;
  file: File;
  name?: string;
}) => {
  const formData = new FormData();
  formData.set('file', file);
  if (name?.trim()) {
    formData.set('name', name.trim());
  }

  return requestAppDev<AppDevProject>(
    `/api/app-dev/spaces/${encodeURIComponent(spaceId)}/projects/import`,
    parseAppDevProject,
    {
      method: 'POST',
      body: formData,
    },
  );
};

export const archiveAppDevProject = async ({
  spaceId,
  projectId,
}: AppDevProjectIdentity) =>
  requestAppDev<{ success: boolean }>(
    buildProjectBasePath({ spaceId, projectId }),
    parseSuccess,
    {
      method: 'DELETE',
      headers: jsonHeaders,
    },
  );

const APP_DEV_EXPORT_CONTENT_TYPES = new Set([
  'application/zip',
  'application/octet-stream',
  'application/x-zip-compressed',
]);
const APP_DEV_RELEASE_CONTENT_TYPES = new Set(['application/zip']);
const MAX_APP_DEV_ARCHIVE_BLOB_BYTES = 128 * 1024 * 1024;

const cancelResponseBody = async (response: Response): Promise<void> => {
  try {
    await response.body?.cancel();
  } catch (error) {
    void error;
    // Discarding a response is best effort; callers still fail closed.
  }
};

const isSafeExportContentDisposition = (value: string | null): boolean => {
  if (!value || value.length > 512 || /[\u0000-\u001f\u007f]/u.test(value)) {
    return false;
  }
  const match =
    /^attachment\s*;\s*filename\s*=\s*(?:"([^"]+)"|([^;\s]+))\s*$/iu.exec(
      value.trim(),
    );
  const filename = match?.[1] ?? match?.[2];
  return Boolean(
    filename &&
      filename.length <= 128 &&
      filename === filename.trim() &&
      !/(?:\.\.|[\\/:?#]|https?:|token=|bearer\s)/iu.test(filename),
  );
};

const parseSafeArchiveContentLength = (
  value: string | null,
): number | undefined => {
  if (value === null) {
    return undefined;
  }
  if (!/^(?:0|[1-9]\d*)$/u.test(value)) {
    throw new AppDevSafeError('invalid_response');
  }
  const length = Number(value);
  if (
    !Number.isSafeInteger(length) ||
    length < 0 ||
    length > MAX_APP_DEV_ARCHIVE_BLOB_BYTES
  ) {
    throw new AppDevSafeError('invalid_response');
  }
  return length;
};

const readSafeAppDevArchiveBlob = async (
  response: Response,
  allowedContentTypes: ReadonlySet<string>,
  signal?: AbortSignal,
): Promise<Blob> => {
  const contentType =
    response.headers
      .get('content-type')
      ?.split(';', 1)[0]
      ?.trim()
      .toLowerCase() ?? '';
  let expectedLength: number | undefined;
  try {
    expectedLength = parseSafeArchiveContentLength(
      response.headers.get('content-length'),
    );
  } catch {
    await cancelResponseBody(response);
    throw new AppDevSafeError('invalid_response');
  }
  if (
    !allowedContentTypes.has(contentType) ||
    !isSafeExportContentDisposition(response.headers.get('content-disposition'))
  ) {
    await cancelResponseBody(response);
    throw new AppDevSafeError('invalid_response');
  }

  if (signal?.aborted) {
    await cancelResponseBody(response);
    throw new AppDevSafeError('unavailable');
  }

  const body = response.body as
    | (ReadableStream<Uint8Array> & {
        getReader?: () => ReadableStreamDefaultReader<Uint8Array>;
      })
    | null;
  if (!body || typeof body.getReader !== 'function') {
    await cancelResponseBody(response);
    throw new AppDevSafeError('invalid_response');
  }

  const reader = body.getReader();
  const chunks: Uint8Array[] = [];
  let total = 0;
  let cancelPromise: Promise<void> | undefined;
  const cancelReader = async () => {
    cancelPromise ??= (async () => {
      try {
        await reader.cancel();
      } catch (error) {
        void error;
        // The response remains rejected even when a backend reader ignores cancel.
      }
    })();
    await cancelPromise;
  };
  const readChunk = () => {
    if (!signal) {
      return reader.read();
    }
    return new Promise<ReadableStreamReadResult<Uint8Array>>(
      (resolve, reject) => {
        const onAbort = () => {
          void cancelReader();
          reject(new AppDevSafeError('unavailable'));
        };
        if (signal.aborted) {
          onAbort();
          return;
        }
        signal.addEventListener('abort', onAbort, { once: true });
        reader
          .read()
          .then(resolve, reject)
          .finally(() => {
            signal.removeEventListener('abort', onAbort);
          });
      },
    );
  };

  try {
    while (true) {
      const part = await readChunk();
      if (part.done) {
        break;
      }
      if (!(part.value instanceof Uint8Array)) {
        await cancelReader();
        throw new AppDevSafeError('invalid_response');
      }
      total += part.value.byteLength;
      if (
        total > MAX_APP_DEV_ARCHIVE_BLOB_BYTES ||
        (expectedLength !== undefined && total > expectedLength)
      ) {
        await cancelReader();
        throw new AppDevSafeError('invalid_response');
      }
      chunks.push(part.value);
    }
  } catch (error) {
    await cancelReader();
    if (error instanceof AppDevSafeError) {
      throw error;
    }
    throw new AppDevSafeError(
      signal?.aborted ? 'unavailable' : 'invalid_response',
    );
  } finally {
    try {
      reader.releaseLock();
    } catch (error) {
      void error;
      // A released/failed reader has no remaining capability to retain.
    }
  }
  if (expectedLength !== undefined && total !== expectedLength) {
    await cancelReader();
    await cancelResponseBody(response);
    throw new AppDevSafeError('invalid_response');
  }
  return new Blob(chunks, { type: contentType });
};

const requestSafeAppDevArchiveBlob = async (
  path: string,
  options: {
    allowedContentTypes: ReadonlySet<string>;
    rejectStale?: boolean;
    signal?: AbortSignal;
  },
): Promise<Blob> => {
  let response: Response;
  try {
    response = await fetch(path, {
      credentials: 'include',
      signal: options.signal,
    });
  } catch {
    throw new AppDevSafeError('unavailable');
  }

  if (
    options.rejectStale &&
    response.headers.get('x-appdev-release-stale')?.trim().toLowerCase() ===
      'true'
  ) {
    await cancelResponseBody(response);
    throw new AppDevSafeError('release_stale');
  }
  if (!response.ok) {
    await cancelResponseBody(response);
    throw safeErrorForStatus(response.status);
  }
  return readSafeAppDevArchiveBlob(
    response,
    options.allowedContentTypes,
    options.signal,
  );
};

export const exportAppDevProject = async ({
  spaceId,
  projectId,
  signal,
}: AppDevProjectIdentity & { signal?: AbortSignal }) =>
  requestSafeAppDevArchiveBlob(
    `${buildProjectBasePath({ spaceId, projectId })}/export`,
    { allowedContentTypes: APP_DEV_EXPORT_CONTENT_TYPES, signal },
  );

export const buildAppDevProject = async ({
  spaceId,
  projectId,
  operationId,
  signal,
}: AppDevProjectIdentity & {
  operationId: string;
  signal?: AbortSignal;
}) =>
  requestProvider<AppDevBuildInfo>(
    `${buildProjectBasePath({ spaceId, projectId })}/build`,
    parseProviderBuild,
    {
      method: 'POST',
      headers: providerOperationHeaders(operationId),
      signal,
    },
  );

export const getAppDevBuildStatus = async ({
  spaceId,
  projectId,
  operationId,
  expectedGeneration,
  signal,
}: AppDevProjectIdentity & {
  operationId: string;
  expectedGeneration: number;
  signal?: AbortSignal;
}) => {
  const result = await requestProvider<AppDevBuildInfo>(
    `${buildProjectBasePath({ spaceId, projectId })}/build`,
    parseProviderBuild,
    {
      method: 'GET',
      headers: providerOperationHeaders(operationId),
      signal,
    },
  );
  if (result.generation !== expectedGeneration) {
    throw new AppDevSafeError('conflict');
  }
  return result;
};

export const downloadAppDevRelease = async ({
  spaceId,
  projectId,
  signal,
}: AppDevProjectIdentity & { signal?: AbortSignal }) =>
  requestSafeAppDevArchiveBlob(
    `${buildProjectBasePath({ spaceId, projectId })}/release`,
    {
      allowedContentTypes: APP_DEV_RELEASE_CONTENT_TYPES,
      rejectStale: true,
      signal,
    },
  );

export const getAppDevProject = async ({
  spaceId,
  projectId,
}: AppDevProjectIdentity) =>
  requestAppDev<AppDevProject>(
    buildProjectBasePath({ spaceId, projectId }),
    parseAppDevProject,
  );

export const updateAppDevProject = async ({
  spaceId,
  projectId,
  name,
  description,
}: AppDevProjectIdentity & {
  name: string;
  description?: string;
}) =>
  requestAppDev<AppDevProject>(
    buildProjectBasePath({ spaceId, projectId }),
    parseAppDevProject,
    {
      method: 'POST',
      headers: jsonHeaders,
      body: JSON.stringify({ name, description }),
    },
  );

export const duplicateAppDevProject = async ({
  spaceId,
  projectId,
  name,
}: AppDevProjectIdentity & {
  name?: string;
}) =>
  requestAppDev<AppDevProject>(
    `${buildProjectBasePath({ spaceId, projectId })}/duplicate`,
    parseAppDevProject,
    {
      method: 'POST',
      headers: jsonHeaders,
      body: JSON.stringify({ name }),
    },
  );

export const listAppDevFiles = async ({
  spaceId,
  projectId,
}: AppDevProjectIdentity) =>
  requestAppDev<{ items: AppDevFileNode[] }>(
    `${buildProjectBasePath({ spaceId, projectId })}/files`,
    value => parseItems(value, parseFileNode),
  );

export const getAppDevFileContent = async ({
  spaceId,
  projectId,
  path,
}: AppDevProjectIdentity & { path: string }) =>
  requestAppDev<AppDevFileContent>(
    `${buildProjectBasePath({ spaceId, projectId })}/files/content${buildQuery({
      path,
    })}`,
    parseFileContent,
  );

export const saveAppDevFileContent = async ({
  spaceId,
  projectId,
  path,
  content,
  version,
}: AppDevProjectIdentity & {
  path: string;
  content: string;
  version?: string;
}) =>
  requestAppDev<AppDevFileContent>(
    `${buildProjectBasePath({ spaceId, projectId })}/files/content`,
    parseFileContent,
    {
      method: 'POST',
      headers: jsonHeaders,
      body: JSON.stringify({ path, content, version }),
    },
  );

export const uploadAppDevFiles = async ({
  spaceId,
  projectId,
  files,
  filePaths,
}: AppDevProjectIdentity & {
  files: File[];
  filePaths: string[];
}) => {
  const formData = new FormData();

  files.forEach(file => formData.append('files', file));
  filePaths.forEach(filePath => formData.append('filePaths', filePath));

  return requestAppDev<{ uploaded: number }>(
    `${buildProjectBasePath({ spaceId, projectId })}/files/upload`,
    parseUploaded,
    {
      method: 'POST',
      body: formData,
    },
  );
};

export const deleteAppDevFile = async ({
  spaceId,
  projectId,
  path,
}: AppDevProjectIdentity & { path: string }) =>
  requestAppDev<{ success: boolean }>(
    `${buildProjectBasePath({ spaceId, projectId })}/files`,
    parseSuccess,
    {
      method: 'DELETE',
      headers: jsonHeaders,
      body: JSON.stringify({ path }),
    },
  );

export const renameAppDevFile = async ({
  spaceId,
  projectId,
  sourcePath,
  targetPath,
}: AppDevProjectIdentity & {
  sourcePath: string;
  targetPath: string;
}) =>
  requestAppDev<{ success: boolean }>(
    `${buildProjectBasePath({ spaceId, projectId })}/files/rename`,
    parseSuccess,
    {
      method: 'POST',
      headers: jsonHeaders,
      body: JSON.stringify({ sourcePath, targetPath }),
    },
  );

export const listAppDevSnapshots = async ({
  spaceId,
  projectId,
  signal,
}: AppDevProjectIdentity & { signal?: AbortSignal }) =>
  requestAppDev<{ items: AppDevSnapshot[] }>(
    `${buildProjectBasePath({ spaceId, projectId })}/snapshots`,
    value => parseItems(value, parseSnapshot),
    { signal },
  );

export const createAppDevSnapshot = async ({
  spaceId,
  projectId,
  label,
}: AppDevProjectIdentity & { label?: string }) =>
  requestAppDev<AppDevSnapshot>(
    `${buildProjectBasePath({ spaceId, projectId })}/snapshots`,
    parseSnapshot,
    {
      method: 'POST',
      headers: jsonHeaders,
      body: JSON.stringify({ label }),
    },
  );

export const restoreAppDevSnapshot = async ({
  spaceId,
  projectId,
  snapshotId,
  operationId,
  signal,
}: AppDevProjectIdentity & {
  snapshotId: string;
  operationId: string;
  signal?: AbortSignal;
}) => {
  const normalizedSnapshotId = normalizeAppDevSnapshotID(snapshotId);
  if (!normalizedSnapshotId) {
    throw new AppDevSafeError('invalid_request');
  }
  return requestProvider<AppDevRuntimeInfo>(
    `${buildProjectBasePath({
      spaceId,
      projectId,
    })}/snapshots/${encodeURIComponent(normalizedSnapshotId)}/restore`,
    parseProviderRuntime,
    {
      method: 'POST',
      headers: providerOperationHeaders(operationId),
      signal,
    },
  );
};

export const getAppDevRuntimeStatus = async ({
  spaceId,
  projectId,
  signal,
}: AppDevProjectIdentity & { signal?: AbortSignal }) =>
  requestProvider<AppDevRuntimeInfo>(
    `${buildProjectBasePath({ spaceId, projectId })}/runtime/status`,
    parseProviderRuntime,
    { signal },
  );

export const startAppDevRuntime = async ({
  spaceId,
  projectId,
  operationId,
  signal,
}: AppDevProjectIdentity & { operationId: string; signal?: AbortSignal }) =>
  requestProvider<AppDevRuntimeInfo>(
    `${buildProjectBasePath({ spaceId, projectId })}/runtime/start`,
    parseProviderRuntime,
    {
      method: 'POST',
      headers: providerOperationHeaders(operationId),
      signal,
    },
  );

export const keepAliveAppDevRuntime = async ({
  spaceId,
  projectId,
  signal,
}: AppDevProjectIdentity & { signal?: AbortSignal }) =>
  requestProvider<AppDevRuntimeInfo>(
    `${buildProjectBasePath({ spaceId, projectId })}/runtime/keep-alive`,
    parseProviderRuntime,
    {
      method: 'POST',
      signal,
    },
  );

export const restartAppDevRuntime = async ({
  spaceId,
  projectId,
  operationId,
  signal,
}: AppDevProjectIdentity & { operationId: string; signal?: AbortSignal }) =>
  requestProvider<AppDevRuntimeInfo>(
    `${buildProjectBasePath({ spaceId, projectId })}/runtime/restart`,
    parseProviderRuntime,
    {
      method: 'POST',
      headers: providerOperationHeaders(operationId),
      signal,
    },
  );

export const stopAppDevRuntime = async ({
  spaceId,
  projectId,
  operationId,
  signal,
}: AppDevProjectIdentity & { operationId: string; signal?: AbortSignal }) =>
  requestProvider<AppDevRuntimeInfo>(
    `${buildProjectBasePath({ spaceId, projectId })}/runtime/stop`,
    parseProviderRuntime,
    {
      method: 'POST',
      headers: providerOperationHeaders(operationId),
      signal,
    },
  );

const APP_DEV_RUNTIME_LOG_STATES = new Set<AppDevRuntimeStatus>([
  'stopped',
  'starting',
  'running',
  'recovering',
  'stopping',
  'cleanup_pending',
  'error',
]);

const isAppDevRuntimeStatus = (value: unknown): value is AppDevRuntimeStatus =>
  typeof value === 'string' &&
  APP_DEV_RUNTIME_LOG_STATES.has(value as AppDevRuntimeStatus);

const stableLogHash = (value: string): string => {
  let hash = 0x811c9dc5;
  for (let index = 0; index < value.length; index += 1) {
    hash ^= value.charCodeAt(index);
    hash = Math.imul(hash, 0x01000193);
  }
  return (hash >>> 0).toString(16).padStart(8, '0');
};

export const listAppDevRuntimeLogs = async ({
  spaceId,
  projectId,
  signal,
}: AppDevProjectIdentity & { signal?: AbortSignal }) =>
  requestProvider<{
    items: AppDevRuntimeLog[];
    state?: AppDevRuntimeStatus;
    safeMessage?: string;
  }>(
    `${buildProjectBasePath({ spaceId, projectId })}/runtime/logs`,
    payload => {
      const value = requireProviderObject(payload);
      if (!isAppDevRuntimeStatus(value.state) || !Array.isArray(value.logs)) {
        throw new Error('invalid logs payload');
      }
      const occurrences = new Map<string, number>();
      const usedIDs = new Set<string>();
      const logs = value.logs.map(line => {
        const message = safeProviderMessage(line);
        if (!message) {
          throw new Error('invalid log line');
        }
        const occurrence = occurrences.get(message) ?? 0;
        occurrences.set(message, occurrence + 1);
        const baseID = `provider-log-${stableLogHash(message)}-${occurrence}`;
        let id = baseID;
        let collision = 0;
        while (usedIDs.has(id)) {
          collision += 1;
          id = `${baseID}-${collision}`;
        }
        usedIDs.add(id);
        return {
          id,
          level: 'info' as const,
          message,
          timestamp: '',
        };
      });
      return {
        items: logs,
        state: value.state,
        safeMessage: safeProviderMessage(value.safe_message),
      };
    },
    { signal },
  );

export const listAppDevModels = async ({ spaceId }: { spaceId: string }) =>
  requestAppDev<{ items: Array<AppDevModel & Record<string, unknown>> }>(
    `/api/app-dev/spaces/${encodeURIComponent(
      spaceId,
    )}/models${buildQuery({ scenario: 'PageApp' })}`,
    parseRecordItems,
  ).then(result => ({
    ...result,
    items: (result.items || [])
      .map(model => ({
        id: String(model.id || ''),
        name: String(model.name || '未命名模型'),
        provider:
          typeof model.provider === 'string' ? model.provider : undefined,
        protocol:
          typeof model.protocol === 'string' ? model.protocol : undefined,
        supportsMultiModal: Boolean(
          model.supportsMultiModal ?? model.supports_multi_modal,
        ),
        supportsImageUnderstanding: Boolean(
          model.supportsImageUnderstanding ??
            model.supports_image_understanding,
        ),
        enableBase64URL: Boolean(
          model.enableBase64URL ?? model.enable_base64_url,
        ),
      }))
      .filter(model => model.id),
  }));

export const listAppDevDataSources = async ({
  spaceId,
}: {
  spaceId: string;
}) => {
  const response = await fetch('/api/knowledge/list', {
    method: 'POST',
    headers: jsonHeaders,
    body: JSON.stringify({
      space_id: spaceId,
      page: 1,
      size: 50,
    }),
  });
  const payload = (await readJson<unknown>(response)) as {
    code?: number;
    message?: string;
    msg?: string;
    data?: {
      dataset_list?: Array<Record<string, unknown>>;
      items?: Array<Record<string, unknown>>;
      list?: Array<Record<string, unknown>>;
    };
    dataset_list?: Array<Record<string, unknown>>;
    items?: Array<Record<string, unknown>>;
    list?: Array<Record<string, unknown>>;
  };
  if (!response.ok || (payload.code !== undefined && payload.code !== 0)) {
    throw new Error(
      payload.message || payload.msg || `数据源加载失败：${response.status}`,
    );
  }

  const datasets =
    payload.data?.dataset_list ||
    payload.data?.items ||
    payload.data?.list ||
    payload.dataset_list ||
    payload.items ||
    payload.list ||
    [];
  const items = datasets
    .map(dataset => ({
      id: String(dataset.dataset_id || dataset.id || ''),
      name: String(dataset.name || dataset.dataset_name || '未命名数据源'),
      type: String(dataset.format_type || dataset.type || 'knowledge'),
      description: String(dataset.description || ''),
    }))
    .filter(dataset => dataset.id) as AppDevDataSource[];

  return {
    items,
    total: items.length,
  };
};

export const buildAppDevEventsUrl = ({
  spaceId,
  projectId,
}: AppDevProjectIdentity) =>
  `${buildProjectBasePath({ spaceId, projectId })}/chat/events`;

export const sendAppDevChatMessage = async ({
  spaceId,
  projectId,
  message,
  modelId,
  dataSources,
  attachments,
}: AppDevProjectIdentity & {
  message: string;
  modelId?: string;
  dataSources?: AppDevDataSource[];
  attachments?: AppDevChatAttachment[];
}) =>
  requestAppDev<AppDevChatSession>(
    `${buildProjectBasePath({ spaceId, projectId })}/chat`,
    parseChatSession,
    {
      method: 'POST',
      headers: jsonHeaders,
      body: JSON.stringify({ message, modelId, dataSources, attachments }),
    },
  );

export const cancelAppDevChat = async ({
  spaceId,
  projectId,
}: AppDevProjectIdentity) =>
  requestAppDev<AppDevChatStatus>(
    `${buildProjectBasePath({ spaceId, projectId })}/chat/cancel`,
    parseChatStatus,
    {
      method: 'POST',
      headers: jsonHeaders,
    },
  );

export const listAppDevChatHistory = async ({
  spaceId,
  projectId,
}: AppDevProjectIdentity) =>
  requestAppDev<{ items: AppDevChatMessage[] }>(
    `${buildProjectBasePath({ spaceId, projectId })}/chat/history`,
    value => parseItems(value, parseChatMessage),
  );

export const getAppDevChatStatus = async ({
  spaceId,
  projectId,
}: AppDevProjectIdentity) =>
  requestAppDev<AppDevChatStatus>(
    `${buildProjectBasePath({ spaceId, projectId })}/chat/status`,
    parseChatStatus,
  );
