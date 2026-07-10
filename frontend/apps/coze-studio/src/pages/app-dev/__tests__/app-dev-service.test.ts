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

import { describe, expect, it, vi, beforeEach } from 'vitest';

import {
  archiveAppDevProject,
  buildAppDevEventsUrl,
  createAppDevProject,
  duplicateAppDevProject,
  exportAppDevProject,
  getAppDevProject,
  listAppDevDataSources,
  listAppDevModels,
  listAppDevProjects,
  normalizeAppDevError,
  sendAppDevChatMessage,
  updateAppDevProject,
} from '../service';

describe('app-dev service', () => {
  beforeEach(() => {
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

  it('turns http errors into readable messages', async () => {
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
    ).rejects.toThrow('无权限访问项目');
  });

  it('keeps workspace development policy errors distinct from login expiry', () => {
    expect(
      normalizeAppDevError(
        new Error('unauthorized access : workspace does not allow development'),
      ),
    ).toBe('当前工作空间未开启网页应用开发，请在“工作空间”设置中开启后重试');
  });

  it('continues to normalize missing sessions as login expiry', () => {
    expect(normalizeAppDevError(new Error('missing user session'))).toBe(
      '登录状态已失效，请重新登录后重试',
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
    vi.spyOn(globalThis, 'fetch').mockResolvedValueOnce(
      new Response('zip-content', {
        status: 200,
        headers: {
          'content-type': 'application/zip',
        },
      }),
    );

    const blob = await exportAppDevProject({ spaceId: 's1', projectId: 'p1' });
    expect(blob).toBeInstanceOf(Blob);
  });

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

  it('normalizes unknown errors', () => {
    expect(normalizeAppDevError(new Error('network down'))).toBe(
      'network down',
    );
    expect(normalizeAppDevError('bad')).toBe('bad');
    expect(normalizeAppDevError(null)).toBe('请求失败，请稍后重试');
  });
});
