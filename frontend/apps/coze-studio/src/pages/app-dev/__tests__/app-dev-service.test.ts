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

/* eslint-disable @typescript-eslint/require-await -- Response test doubles preserve async browser contracts. */

import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';

import {
  APP_DEV_BUILD_SAFE_ERROR_CODES,
  type AppDevBuildSafeErrorCode,
} from '../types';
import {
  AppDevSafeError,
  archiveAppDevProject,
  buildAppDevProject,
  buildAppDevEventsUrl,
  createAppDevProject,
  downloadAppDevRelease,
  duplicateAppDevProject,
  exportAppDevProject,
  getAppDevBuildStatus,
  getAppDevProject,
  getAppDevRuntimeStatus,
  keepAliveAppDevRuntime,
  listAppDevFiles,
  listAppDevDataSources,
  listAppDevModels,
  listAppDevProjects,
  listAppDevRuntimeLogs,
  listAppDevSnapshots,
  normalizeAppDevError,
  restartAppDevRuntime,
  restoreAppDevSnapshot,
  sendAppDevChatMessage,
  startAppDevRuntime,
  stopAppDevRuntime,
  updateAppDevProject,
} from '../service';

const streamingArchiveResponse = (
  content: string,
  headers: Record<string, string>,
) => {
  const bytes = new TextEncoder().encode(content);
  let delivered = false;
  const read = vi.fn().mockImplementation(async () => {
    if (delivered) {
      return { done: true, value: undefined };
    }
    delivered = true;
    return { done: false, value: bytes };
  });
  const readerCancel = vi.fn().mockResolvedValue(undefined);
  const bodyCancel = vi.fn().mockResolvedValue(undefined);
  const normalized = new Map(
    Object.entries(headers).map(([key, value]) => [key.toLowerCase(), value]),
  );
  return {
    read,
    readerCancel,
    bodyCancel,
    response: {
      ok: true,
      status: 200,
      headers: {
        get: (name: string) => normalized.get(name.toLowerCase()) ?? null,
      },
      body: {
        cancel: bodyCancel,
        getReader: () => ({
          read,
          cancel: readerCancel,
          releaseLock: vi.fn(),
        }),
      },
      blob: vi.fn(() => {
        throw new Error('streaming contract must not call blob');
      }),
    } as unknown as Response,
  };
};

describe('app-dev service', () => {
  beforeEach(() => {
    vi.restoreAllMocks();
  });
  afterEach(() => {
    vi.restoreAllMocks();
  });

  it('normalizes project list response', async () => {
    vi.spyOn(globalThis, 'fetch').mockResolvedValueOnce(
      new Response(
        JSON.stringify({
          code: 0,
          data: {
            items: [{ id: 'p1', name: '商城首页', status: 'ready' }],
            total: 1,
          },
        }),
        {
          headers: {
            'content-type': 'application/json',
          },
        },
      ),
    );

    await expect(listAppDevProjects({ spaceId: 's1' })).resolves.toEqual({
      items: [{ id: 'p1', name: '商城首页', status: 'ready' }],
      total: 1,
    });
  });

  it('sends project creation payload without user controlled owner fields', async () => {
    const fetchMock = vi.spyOn(globalThis, 'fetch').mockResolvedValueOnce(
      new Response(
        JSON.stringify({
          code: 0,
          data: { id: 'p2', name: '活动页', status: 'creating' },
        }),
        {
          headers: {
            'content-type': 'application/json',
          },
        },
      ),
    );

    await createAppDevProject({
      spaceId: 's1',
      name: '活动页',
      prompt: '做一个活动落地页',
    });

    expect(fetchMock).toHaveBeenCalledWith(
      '/api/app-dev/spaces/s1/projects',
      expect.objectContaining({
        method: 'POST',
        body: JSON.stringify({
          name: '活动页',
          prompt: '做一个活动落地页',
        }),
      }),
    );
  });

  it('maps http errors to a fixed safe message', async () => {
    vi.spyOn(globalThis, 'fetch').mockResolvedValueOnce(
      new Response(JSON.stringify({ message: '无权限访问项目' }), {
        status: 403,
        headers: {
          'content-type': 'application/json',
        },
      }),
    );

    await expect(
      getAppDevProject({ spaceId: 's1', projectId: 'p1' }),
    ).rejects.toThrow('当前账号无权执行此操作');
  });

  it('does not classify raw error strings as trusted safe errors', () => {
    expect(
      normalizeAppDevError(
        new Error('unauthorized access : workspace does not allow development'),
      ),
    ).toBe('请求失败，请稍后重试');
    expect(normalizeAppDevError(new Error('missing user session'))).toBe(
      '请求失败，请稍后重试',
    );
  });

  it('builds the SSE events URL from safe route identities', () => {
    expect(
      buildAppDevEventsUrl({
        spaceId: 'space 1',
        projectId: 'project/1',
      }),
    ).toBe('/api/app-dev/spaces/space%201/projects/project%2F1/chat/events');
  });

  it('loads PageApp models through the appdev model endpoint', async () => {
    const fetchMock = vi.spyOn(globalThis, 'fetch').mockResolvedValueOnce(
      new Response(
        JSON.stringify({
          code: 0,
          data: {
            items: [
              {
                id: '1',
                name: 'GPT',
                provider: 'OpenAI',
                supports_multi_modal: true,
                supports_image_understanding: true,
                enable_base64_url: true,
              },
            ],
            total: 1,
          },
        }),
        {
          headers: {
            'content-type': 'application/json',
          },
        },
      ),
    );

    await expect(listAppDevModels({ spaceId: 's1' })).resolves.toEqual({
      items: [
        {
          id: '1',
          name: 'GPT',
          provider: 'OpenAI',
          protocol: undefined,
          supportsMultiModal: true,
          supportsImageUnderstanding: true,
          enableBase64URL: true,
        },
      ],
      total: 1,
    });
    expect(fetchMock).toHaveBeenCalledWith(
      '/api/app-dev/spaces/s1/models?scenario=PageApp',
      undefined,
    );
  });

  it('archives a project without sending owner fields', async () => {
    const fetchMock = vi.spyOn(globalThis, 'fetch').mockResolvedValueOnce(
      new Response(
        JSON.stringify({
          code: 0,
          data: { success: true },
        }),
        {
          headers: {
            'content-type': 'application/json',
          },
        },
      ),
    );

    await archiveAppDevProject({ spaceId: 's1', projectId: 'p1' });
    expect(fetchMock).toHaveBeenCalledWith(
      '/api/app-dev/spaces/s1/projects/p1',
      expect.objectContaining({
        method: 'DELETE',
      }),
    );
  });

  it('updates project metadata without sending owner fields', async () => {
    const fetchMock = vi.spyOn(globalThis, 'fetch').mockResolvedValueOnce(
      new Response(
        JSON.stringify({
          code: 0,
          data: { id: 'p1', name: '新版活动页', status: 'ready' },
        }),
        {
          headers: {
            'content-type': 'application/json',
          },
        },
      ),
    );

    await updateAppDevProject({
      spaceId: 's1',
      projectId: 'p1',
      name: '新版活动页',
      description: '已改名',
    });

    expect(fetchMock).toHaveBeenCalledWith(
      '/api/app-dev/spaces/s1/projects/p1',
      expect.objectContaining({
        method: 'POST',
        body: JSON.stringify({
          name: '新版活动页',
          description: '已改名',
        }),
      }),
    );
  });

  it('duplicates a project without sending owner fields', async () => {
    const fetchMock = vi.spyOn(globalThis, 'fetch').mockResolvedValueOnce(
      new Response(
        JSON.stringify({
          code: 0,
          data: { id: 'p2', name: '活动页副本', status: 'ready' },
        }),
        {
          headers: {
            'content-type': 'application/json',
          },
        },
      ),
    );

    await duplicateAppDevProject({
      spaceId: 's1',
      projectId: 'p1',
      name: '活动页副本',
    });

    expect(fetchMock).toHaveBeenCalledWith(
      '/api/app-dev/spaces/s1/projects/p1/duplicate',
      expect.objectContaining({
        method: 'POST',
        body: JSON.stringify({
          name: '活动页副本',
        }),
      }),
    );
  });

  it('downloads project export as a blob', async () => {
    const fixture = streamingArchiveResponse('zip-content', {
      'content-type': 'application/zip',
      'content-disposition': 'attachment; filename="appdev-export.zip"',
      'content-length': '11',
    });
    vi.spyOn(globalThis, 'fetch').mockResolvedValueOnce(fixture.response);

    const blob = await exportAppDevProject({ spaceId: 's1', projectId: 'p1' });
    expect(blob).toBeInstanceOf(Blob);
    expect(blob.size).toBe(11);
    expect(fixture.read).toHaveBeenCalledTimes(2);
    expect(fixture.readerCancel).not.toHaveBeenCalled();
  });

  it('does not read or expose an unsuccessful export response body', async () => {
    const raw =
      'https://provider.invalid/export?token=raw-secret Bearer raw-api-key';
    const cancel = vi.fn().mockResolvedValue(undefined);
    const json = vi.fn().mockResolvedValue({ message: raw });
    const blob = vi.fn().mockResolvedValue(new Blob([raw]));
    const response = {
      ok: false,
      status: 503,
      headers: new Headers(),
      body: { cancel },
      json,
      blob,
    } as unknown as Response;
    vi.spyOn(globalThis, 'fetch').mockResolvedValueOnce(response);

    const error = await exportAppDevProject({
      spaceId: 's1',
      projectId: 'p1',
    }).catch(caught => caught);

    expect(error).toBeInstanceOf(AppDevSafeError);
    expect(error).toMatchObject({ code: 'unavailable' });
    expect(String(error)).not.toMatch(/provider\.invalid|raw-secret|Bearer/u);
    expect(cancel).toHaveBeenCalledTimes(1);
    expect(json).not.toHaveBeenCalled();
    expect(blob).not.toHaveBeenCalled();
  });

  it('normalizes export transport failures without exposing raw errors', async () => {
    vi.spyOn(globalThis, 'fetch').mockRejectedValueOnce(
      new Error('https://provider.invalid?token=raw-secret'),
    );

    const error = await exportAppDevProject({
      spaceId: 's1',
      projectId: 'p1',
    }).catch(caught => caught);

    expect(error).toBeInstanceOf(AppDevSafeError);
    expect(error).toMatchObject({ code: 'unavailable' });
    expect(String(error)).not.toContain('raw-secret');
  });

  it.each([
    ['text/html', 'attachment; filename="appdev-export.zip"'],
    ['application/json', 'attachment; filename="appdev-export.zip"'],
    ['application/zip', 'inline; filename="appdev-export.zip"'],
    ['application/zip', 'attachment; filename="../secret.zip"'],
  ])(
    'rejects unsafe export metadata (%s, %s) before reading the blob',
    async (contentType, contentDisposition) => {
      const cancel = vi.fn().mockResolvedValue(undefined);
      const blob = vi.fn().mockResolvedValue(new Blob(['not-a-safe-archive']));
      const response = {
        ok: true,
        status: 200,
        headers: new Headers({
          'content-type': contentType,
          'content-disposition': contentDisposition,
        }),
        body: { cancel },
        blob,
      } as unknown as Response;
      vi.spyOn(globalThis, 'fetch').mockResolvedValueOnce(response);

      const error = await exportAppDevProject({
        spaceId: 's1',
        projectId: 'p1',
      }).catch(caught => caught);

      expect(error).toBeInstanceOf(AppDevSafeError);
      expect(error).toMatchObject({ code: 'invalid_response' });
      expect(cancel).toHaveBeenCalledTimes(1);
      expect(blob).not.toHaveBeenCalled();
    },
  );

  it('normalizes knowledge datasets as appdev data sources', async () => {
    vi.spyOn(globalThis, 'fetch').mockResolvedValueOnce(
      new Response(
        JSON.stringify({
          dataset_list: [
            {
              dataset_id: '101',
              name: '产品资料',
              format_type: 'text',
              description: '商品说明',
            },
          ],
        }),
        {
          headers: {
            'content-type': 'application/json',
          },
        },
      ),
    );

    await expect(listAppDevDataSources({ spaceId: 's1' })).resolves.toEqual({
      items: [
        {
          id: '101',
          name: '产品资料',
          type: 'text',
          description: '商品说明',
        },
      ],
      total: 1,
    });
  });

  it('sends selected data sources with chat messages', async () => {
    const fetchMock = vi.spyOn(globalThis, 'fetch').mockResolvedValueOnce(
      new Response(
        JSON.stringify({
          code: 0,
          data: { sessionId: 's', requestId: 'r', running: true },
        }),
        {
          headers: {
            'content-type': 'application/json',
          },
        },
      ),
    );

    await sendAppDevChatMessage({
      spaceId: 's1',
      projectId: 'p1',
      message: '生成页面',
      modelId: 'm1',
      dataSources: [{ id: '101', name: '产品资料', type: 'text' }],
    });

    expect(fetchMock).toHaveBeenCalledWith(
      '/api/app-dev/spaces/s1/projects/p1/chat',
      expect.objectContaining({
        method: 'POST',
        body: JSON.stringify({
          message: '生成页面',
          modelId: 'm1',
          dataSources: [{ id: '101', name: '产品资料', type: 'text' }],
        }),
      }),
    );
  });

  it('sends prototype image attachments with chat messages', async () => {
    const fetchMock = vi.spyOn(globalThis, 'fetch').mockResolvedValueOnce(
      new Response(
        JSON.stringify({
          code: 0,
          data: { sessionId: 's', requestId: 'r', running: true },
        }),
        {
          headers: {
            'content-type': 'application/json',
          },
        },
      ),
    );

    await sendAppDevChatMessage({
      spaceId: 's1',
      projectId: 'p1',
      message: '按原型图生成页面',
      modelId: 'm1',
      attachments: [
        {
          id: 'a1',
          name: 'homepage.png',
          path: 'src/assets/uploads/prototype-homepage.png',
          mimeType: 'image/png',
          size: 1024,
          type: 'prototype_image',
        },
      ],
    });

    expect(fetchMock).toHaveBeenCalledWith(
      '/api/app-dev/spaces/s1/projects/p1/chat',
      expect.objectContaining({
        method: 'POST',
        body: JSON.stringify({
          message: '按原型图生成页面',
          modelId: 'm1',
          attachments: [
            {
              id: 'a1',
              name: 'homepage.png',
              path: 'src/assets/uploads/prototype-homepage.png',
              mimeType: 'image/png',
              size: 1024,
              type: 'prototype_image',
            },
          ],
        }),
      }),
    );
  });

  it('never exposes unknown error strings or provider secrets', () => {
    const malicious =
      'https://provider.example/run?token=secret Bearer abc s3://bucket/object';
    expect(normalizeAppDevError(new Error(malicious))).toBe(
      '请求失败，请稍后重试',
    );
    expect(normalizeAppDevError(malicious)).toBe('请求失败，请稍后重试');
    expect(normalizeAppDevError(null)).toBe('请求失败，请稍后重试');
  });

  it('uses provider runtime routes and explicit idempotency headers', async () => {
    const runtimeWire = {
      generation: 7,
      state: 'running',
      can_start: false,
      recovering: false,
      stopping: false,
      preview_url: 'https://preview.example.test/app',
      safe_message: '运行正常',
    };
    const fetchMock = vi.spyOn(globalThis, 'fetch').mockImplementation(
      async () =>
        new Response(JSON.stringify(runtimeWire), {
          headers: { 'content-type': 'application/json' },
        }),
    );
    const identity = { spaceId: 'space 1', projectId: 'project/1' };

    await expect(getAppDevRuntimeStatus(identity)).resolves.toEqual({
      generation: 7,
      status: 'running',
      canStart: false,
      recovering: false,
      stopping: false,
      previewUrl: 'https://preview.example.test/app',
      message: undefined,
    });
    await startAppDevRuntime({ ...identity, operationId: 'start-operation-1' });
    await stopAppDevRuntime({ ...identity, operationId: 'stop-operation-1' });
    await restartAppDevRuntime({
      ...identity,
      operationId: 'restart-operation-1',
    });
    await keepAliveAppDevRuntime(identity);

    expect(fetchMock.mock.calls.map(([path]) => path)).toEqual([
      '/api/app-dev/spaces/space%201/projects/project%2F1/runtime/status',
      '/api/app-dev/spaces/space%201/projects/project%2F1/runtime/start',
      '/api/app-dev/spaces/space%201/projects/project%2F1/runtime/stop',
      '/api/app-dev/spaces/space%201/projects/project%2F1/runtime/restart',
      '/api/app-dev/spaces/space%201/projects/project%2F1/runtime/keep-alive',
    ]);
    expect(fetchMock.mock.calls.slice(1).map(([, init]) => init)).toEqual(
      expect.arrayContaining([
        expect.objectContaining({
          method: 'POST',
          headers: expect.objectContaining({
            'Idempotency-Key': 'start-operation-1',
          }),
        }),
        expect.objectContaining({
          headers: expect.objectContaining({
            'Idempotency-Key': 'stop-operation-1',
          }),
        }),
        expect.objectContaining({
          headers: expect.objectContaining({
            'Idempotency-Key': 'restart-operation-1',
          }),
        }),
        expect.not.objectContaining({
          headers: expect.objectContaining({
            'Idempotency-Key': expect.anything(),
          }),
        }),
      ]),
    );
  });

  it('rejects a stale release before reading a blob', async () => {
    const cancel = vi.fn().mockResolvedValue(undefined);
    const blob = vi.fn();
    vi.spyOn(globalThis, 'fetch').mockResolvedValueOnce({
      ok: true,
      status: 200,
      headers: new Headers({ 'X-AppDev-Release-Stale': 'true' }),
      body: { cancel },
      blob,
    } as unknown as Response);

    await expect(
      downloadAppDevRelease({ spaceId: 's1', projectId: 'p1' }),
    ).rejects.toThrow('发布产物已过期，请重新构建');
    expect(cancel).toHaveBeenCalledTimes(1);
    expect(blob).not.toHaveBeenCalled();
  });

  it('maps provider error payloads and unknown build codes to fixed safe messages', async () => {
    const malicious =
      'https://provider.example/run?token=secret Bearer abc s3://bucket/object';
    vi.spyOn(globalThis, 'fetch')
      .mockResolvedValueOnce(
        new Response(JSON.stringify({ safe_message: malicious }), {
          status: 503,
          headers: { 'content-type': 'application/json' },
        }),
      )
      .mockResolvedValueOnce(
        new Response(
          JSON.stringify({
            generation: 1,
            state: 'failed',
            release_available: false,
            size: 0,
            stale: false,
            safe_error_code: 'unknown_provider_code',
            safe_message: malicious,
          }),
          { headers: { 'content-type': 'application/json' } },
        ),
      );

    await expect(
      getAppDevRuntimeStatus({ spaceId: 's1', projectId: 'p1' }),
    ).rejects.toThrow('运行服务暂不可用，请稍后重试');
    const build = await buildAppDevProject({
      spaceId: 's1',
      projectId: 'p1',
      operationId: 'build-operation-safe',
    });
    expect(build.safeErrorCode).toBe('build_failed');
    expect(build.safeMessage).toBe('构建失败，请稍后重试');
    expect(JSON.stringify(build)).not.toContain('secret');
  });

  it.each([
    ['malformed JSON', '{"generation":'],
    ['invalid response shape', JSON.stringify({ generation: 'raw-token' })],
  ])('maps %s to a typed invalid-response error', async (_name, body) => {
    vi.spyOn(globalThis, 'fetch').mockResolvedValueOnce(
      new Response(body, {
        headers: { 'content-type': 'application/json' },
      }),
    );
    const error = await getAppDevRuntimeStatus({
      spaceId: 's1',
      projectId: 'p1',
    }).catch(value => value);
    expect(error).toBeInstanceOf(AppDevSafeError);
    expect(error).toMatchObject({
      code: 'invalid_response',
      message: '运行服务返回格式异常，请稍后重试',
    });
    expect(String(error)).not.toContain('raw-token');
  });

  it('maps every backend build safe code to a fixed local message', async () => {
    const messages: Record<AppDevBuildSafeErrorCode, string> = {
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
    let call = 0;
    vi.spyOn(globalThis, 'fetch').mockImplementation(async () => {
      const code = APP_DEV_BUILD_SAFE_ERROR_CODES[call++];
      return new Response(
        JSON.stringify({
          generation: 21,
          state: 'failed',
          release_available: false,
          size: 0,
          stale: false,
          safe_error_code: code,
          safe_message:
            'https://provider.invalid/?token=secret Bearer api-key s3://private/object',
        }),
        { headers: { 'content-type': 'application/json' } },
      );
    });
    for (const code of APP_DEV_BUILD_SAFE_ERROR_CODES) {
      const result = await buildAppDevProject({
        spaceId: 's1',
        projectId: 'p1',
        operationId: `build-operation-${code}`,
      });
      expect(result.safeErrorCode).toBe(code);
      expect(result.safeMessage).toBe(messages[code]);
      expect(JSON.stringify(result)).not.toMatch(/secret|Bearer|s3:\/\//u);
    }
  });

  it('fails closed for malformed generic envelopes and data shapes', async () => {
    const malicious =
      'https://provider.invalid/?token=secret Bearer api-key s3://private/object';
    const cases: Array<{
      response: () => Response;
      request: () => Promise<unknown>;
    }> = [
      {
        response: () => new Response(malicious, { status: 200 }),
        request: () => getAppDevProject({ spaceId: 's1', projectId: 'p1' }),
      },
      {
        response: () =>
          new Response('{"code":0,"data":', {
            headers: { 'content-type': 'application/json' },
          }),
        request: () => getAppDevProject({ spaceId: 's1', projectId: 'p1' }),
      },
      {
        response: () =>
          new Response(JSON.stringify({ code: 0, message: malicious }), {
            headers: { 'content-type': 'application/json' },
          }),
        request: () => getAppDevProject({ spaceId: 's1', projectId: 'p1' }),
      },
      {
        response: () =>
          new Response(
            JSON.stringify({
              code: 0,
              data: { id: 7, name: malicious, status: 'ready' },
            }),
            { headers: { 'content-type': 'application/json' } },
          ),
        request: () => getAppDevProject({ spaceId: 's1', projectId: 'p1' }),
      },
      {
        response: () =>
          new Response(
            JSON.stringify({ code: 0, data: { items: malicious } }),
            { headers: { 'content-type': 'application/json' } },
          ),
        request: () => listAppDevFiles({ spaceId: 's1', projectId: 'p1' }),
      },
    ];

    for (const testCase of cases) {
      vi.spyOn(globalThis, 'fetch').mockResolvedValueOnce(testCase.response());
      const error = await testCase.request().catch(value => value);
      expect(error).toBeInstanceOf(AppDevSafeError);
      expect(error).toMatchObject({ code: 'invalid_response' });
      expect(String(error)).not.toMatch(
        /provider\.invalid|secret|Bearer|s3:\/\//u,
      );
      vi.restoreAllMocks();
    }
  });

  it('rejects malformed provider logs with a typed fixed error', async () => {
    vi.spyOn(globalThis, 'fetch').mockResolvedValueOnce(
      new Response(
        JSON.stringify({
          state: 'running',
          logs: ['safe', { raw: 'https://provider.invalid/?token=secret' }],
        }),
        { headers: { 'content-type': 'application/json' } },
      ),
    );
    const error = await listAppDevRuntimeLogs({
      spaceId: 's1',
      projectId: 'p1',
    }).catch(value => value);
    expect(error).toBeInstanceOf(AppDevSafeError);
    expect(error).toMatchObject({ code: 'invalid_response' });
    expect(String(error)).not.toContain('provider.invalid');
  });

  it('rejects an unknown provider runtime log state', async () => {
    vi.spyOn(globalThis, 'fetch').mockResolvedValueOnce(
      new Response(JSON.stringify({ state: 'provider_running', logs: [] }), {
        headers: { 'content-type': 'application/json' },
      }),
    );

    const error = await listAppDevRuntimeLogs({
      spaceId: 's1',
      projectId: 'p1',
    }).catch(value => value);

    expect(error).toBeInstanceOf(AppDevSafeError);
    expect(error).toMatchObject({ code: 'invalid_response' });
  });

  it('keeps provider log IDs stable while distinguishing duplicate lines', async () => {
    const response = () =>
      new Response(
        JSON.stringify({ state: 'running', logs: ['same', 'other', 'same'] }),
        { headers: { 'content-type': 'application/json' } },
      );
    vi.spyOn(globalThis, 'fetch')
      .mockResolvedValueOnce(response())
      .mockResolvedValueOnce(response());

    const first = await listAppDevRuntimeLogs({
      spaceId: 's1',
      projectId: 'p1',
    });
    const second = await listAppDevRuntimeLogs({
      spaceId: 's1',
      projectId: 'p1',
    });
    const firstIDs = first.items.map(item => item.id);

    expect(second.items.map(item => item.id)).toEqual(firstIDs);
    expect(new Set(firstIDs)).toHaveLength(3);
    expect(first.items[0]?.id).not.toBe(first.items[2]?.id);
  });

  it.each([undefined, '', 'unknown_provider_code'])(
    'maps untrusted build code %s to the generic fixed failure',
    async safeErrorCode => {
      vi.spyOn(globalThis, 'fetch').mockResolvedValueOnce(
        new Response(
          JSON.stringify({
            generation: 22,
            state: 'failed',
            release_available: false,
            size: 0,
            stale: false,
            safe_error_code: safeErrorCode,
            safe_message:
              'https://provider.invalid/?token=secret Bearer s3://object',
          }),
          { headers: { 'content-type': 'application/json' } },
        ),
      );
      await expect(
        buildAppDevProject({
          spaceId: 's1',
          projectId: 'p1',
          operationId: 'build-untrusted-code-operation',
        }),
      ).resolves.toMatchObject({
        safeErrorCode: 'build_failed',
        safeMessage: '构建失败，请稍后重试',
      });
    },
  );

  it('uses the same build key for POST and generation-fenced GET polling', async () => {
    const fetchMock = vi
      .spyOn(globalThis, 'fetch')
      .mockResolvedValueOnce(
        new Response(
          JSON.stringify({
            generation: 11,
            state: 'building',
            release_available: false,
            size: 0,
            updated_at: '2026-07-17T10:00:00Z',
            stale: false,
          }),
          { status: 202, headers: { 'content-type': 'application/json' } },
        ),
      )
      .mockResolvedValueOnce(
        new Response(
          JSON.stringify({
            generation: 11,
            state: 'ready',
            release_available: true,
            size: 42,
            updated_at: '2026-07-17T10:00:01Z',
            stale: false,
          }),
          { headers: { 'content-type': 'application/json' } },
        ),
      );

    await expect(
      buildAppDevProject({
        spaceId: 's1',
        projectId: 'p1',
        operationId: 'build-operation-1',
      }),
    ).resolves.toMatchObject({ generation: 11, state: 'building' });
    await expect(
      getAppDevBuildStatus({
        spaceId: 's1',
        projectId: 'p1',
        operationId: 'build-operation-1',
        expectedGeneration: 11,
      }),
    ).resolves.toMatchObject({
      generation: 11,
      state: 'ready',
      releaseAvailable: true,
    });

    expect(fetchMock).toHaveBeenNthCalledWith(
      1,
      '/api/app-dev/spaces/s1/projects/p1/build',
      expect.objectContaining({
        method: 'POST',
        credentials: 'include',
        headers: expect.objectContaining({
          'Idempotency-Key': 'build-operation-1',
        }),
      }),
    );
    expect(fetchMock).toHaveBeenNthCalledWith(
      2,
      '/api/app-dev/spaces/s1/projects/p1/build',
      expect.objectContaining({
        method: 'GET',
        credentials: 'include',
        headers: expect.objectContaining({
          'Idempotency-Key': 'build-operation-1',
        }),
      }),
    );
  });

  it('uses provider logs, snapshot restore, and authenticated release routes', async () => {
    const releaseFixture = streamingArchiveResponse('zip-content', {
      'content-type': 'application/zip',
      'content-disposition': 'attachment; filename="appdev-release.zip"',
      'content-length': '11',
    });
    const fetchMock = vi
      .spyOn(globalThis, 'fetch')
      .mockResolvedValueOnce(
        new Response(
          JSON.stringify({ state: 'running', logs: ['safe log line'] }),
          { headers: { 'content-type': 'application/json' } },
        ),
      )
      .mockResolvedValueOnce(
        new Response(
          JSON.stringify({
            generation: 8,
            state: 'stopped',
            can_start: true,
            recovering: false,
            stopping: false,
          }),
          { headers: { 'content-type': 'application/json' } },
        ),
      )
      .mockResolvedValueOnce(releaseFixture.response);

    await expect(
      listAppDevRuntimeLogs({ spaceId: 's1', projectId: 'p1' }),
    ).resolves.toMatchObject({
      items: [expect.objectContaining({ message: 'safe log line' })],
    });
    await restoreAppDevSnapshot({
      spaceId: 's1',
      projectId: 'p1',
      snapshotId: 'snapshot-1',
      operationId: 'snapshot-operation-1',
    });
    const release = await downloadAppDevRelease({
      spaceId: 's1',
      projectId: 'p1',
    });
    expect(release).toBeInstanceOf(Blob);
    expect(release.size).toBe(11);
    expect(releaseFixture.read).toHaveBeenCalledTimes(2);
    expect(releaseFixture.readerCancel).not.toHaveBeenCalled();

    expect(fetchMock).toHaveBeenNthCalledWith(
      2,
      '/api/app-dev/spaces/s1/projects/p1/snapshots/snapshot-1/restore',
      expect.objectContaining({
        method: 'POST',
        credentials: 'include',
        headers: expect.objectContaining({
          'Idempotency-Key': 'snapshot-operation-1',
        }),
      }),
    );
    expect(fetchMock).toHaveBeenNthCalledWith(
      3,
      '/api/app-dev/spaces/s1/projects/p1/release',
      expect.objectContaining({ credentials: 'include' }),
    );
  });

  it.each([
    42,
    '',
    ' snapshot-1',
    'snapshot-1 ',
    'snapshot\u0000id',
    'x'.repeat(257),
    'snapshot/1',
    'snapshot\\1',
    'snapshot?1',
  ])(
    'rejects invalid snapshot request identity %j before fetch',
    async snapshotId => {
      const fetchMock = vi.spyOn(globalThis, 'fetch');

      await expect(
        restoreAppDevSnapshot({
          spaceId: 's1',
          projectId: 'p1',
          snapshotId: snapshotId as string,
          operationId: 'snapshot-operation-1',
        }),
      ).rejects.toMatchObject({ code: 'invalid_request' });
      expect(fetchMock).not.toHaveBeenCalled();
    },
  );

  it.each([
    42,
    '',
    ' snapshot-1',
    'snapshot-1 ',
    'snapshot\u0000id',
    'x'.repeat(257),
    'snapshot/1',
    'snapshot\\1',
    'snapshot?1',
  ])('rejects invalid snapshot response identity %j', async snapshotId => {
    const fetchMock = vi.spyOn(globalThis, 'fetch').mockResolvedValueOnce(
      new Response(
        JSON.stringify({
          code: 0,
          data: {
            items: [
              {
                id: snapshotId,
                label: 'Snapshot',
                createdAt: '2026-07-17T00:00:00Z',
              },
            ],
          },
        }),
        { headers: { 'content-type': 'application/json' } },
      ),
    );

    await expect(
      listAppDevSnapshots({ spaceId: 's1', projectId: 'p1' }),
    ).rejects.toMatchObject({ code: 'invalid_response' });
    expect(fetchMock).toHaveBeenCalledTimes(1);
  });

  it('forwards cancellation to the snapshot list request', async () => {
    const controller = new AbortController();
    const fetchMock = vi.spyOn(globalThis, 'fetch').mockResolvedValueOnce(
      new Response(JSON.stringify({ code: 0, data: { items: [] } }), {
        headers: { 'content-type': 'application/json' },
      }),
    );

    await expect(
      listAppDevSnapshots({
        spaceId: 's1',
        projectId: 'p1',
        signal: controller.signal,
      }),
    ).resolves.toEqual({ items: [] });
    expect(fetchMock).toHaveBeenCalledWith(
      '/api/app-dev/spaces/s1/projects/p1/snapshots',
      expect.objectContaining({ signal: controller.signal }),
    );
  });
});
