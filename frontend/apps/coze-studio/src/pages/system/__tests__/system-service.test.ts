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

import { afterEach, describe, expect, it, vi } from 'vitest';

import {
  createAdminModel,
  deleteAdminModel,
  getAdminBasicConfig,
  getAdminKnowledgeConfig,
  getAdminModelList,
  getSystemAdminStatus,
  listAdminUserSpaces,
  listAdminWorkspaceMembers,
  listAdminUsers,
  listAdminWorkspaces,
} from '../service';

describe('system service', () => {
  const previousFetch = globalThis.fetch;

  afterEach(() => {
    globalThis.fetch = previousFetch;
    vi.restoreAllMocks();
  });

  it('returns true when admin status endpoint allows the request', async () => {
    const fetchMock = vi.fn().mockResolvedValue({
      ok: true,
      status: 200,
      json: async () => ({
        data: {
          is_admin: true,
        },
      }),
    });
    globalThis.fetch = fetchMock as never;

    await expect(getSystemAdminStatus()).resolves.toEqual({
      is_admin: true,
    });
    expect(fetchMock).toHaveBeenCalledWith('/api/admin/auth/status', {
      credentials: 'include',
    });
  });

  it('returns false when admin status endpoint rejects the request', async () => {
    globalThis.fetch = vi.fn().mockResolvedValue({
      ok: false,
      status: 401,
      json: async () => ({}),
    }) as never;

    await expect(getSystemAdminStatus()).resolves.toEqual({
      is_admin: false,
    });
  });

  it('posts admin workspace list filters', async () => {
    const fetchMock = vi.fn().mockResolvedValue({
      ok: true,
      status: 200,
      json: async () => ({
        workspaces: [
          {
            id: '101',
            name: '畅享 AI',
            owner_name: 'Owner',
            total_member_num: 2,
          },
        ],
        total: 1,
      }),
    });
    globalThis.fetch = fetchMock as never;

    await expect(
      listAdminWorkspaces({
        keyword: '畅享',
        page: 1,
        size: 20,
      }),
    ).resolves.toMatchObject({
      total: 1,
      workspaces: [
        {
          id: '101',
          name: '畅享 AI',
        },
      ],
    });
    expect(fetchMock).toHaveBeenCalledWith('/api/admin/workspaces/list', {
      body: JSON.stringify({
        keyword: '畅享',
        page: 1,
        size: 20,
      }),
      credentials: 'include',
      headers: {
        'content-type': 'application/json',
      },
      method: 'POST',
    });
  });

  it('posts admin user list filters', async () => {
    const fetchMock = vi.fn().mockResolvedValue({
      ok: true,
      status: 200,
      json: async () => ({
        users: [
          {
            user_id: '9',
            name: 'Owner',
            email: 'owner@example.test',
          },
        ],
        total: 1,
      }),
    });
    globalThis.fetch = fetchMock as never;

    await expect(
      listAdminUsers({
        keyword: 'owner',
        page: 1,
        size: 20,
      }),
    ).resolves.toMatchObject({
      total: 1,
      users: [
        {
          user_id: '9',
          email: 'owner@example.test',
        },
      ],
    });
    expect(fetchMock).toHaveBeenCalledWith('/api/admin/users/list', {
      body: JSON.stringify({
        keyword: 'owner',
        page: 1,
        size: 20,
      }),
      credentials: 'include',
      headers: {
        'content-type': 'application/json',
      },
      method: 'POST',
    });
  });

  it('posts admin workspace member lookup', async () => {
    const fetchMock = vi.fn().mockResolvedValue({
      ok: true,
      status: 200,
      json: async () => ({
        members: [
          {
            user_id: '9',
            name: 'Owner',
            email: 'owner@example.test',
            role_type: 2,
          },
        ],
      }),
    });
    globalThis.fetch = fetchMock as never;

    await expect(
      listAdminWorkspaceMembers({
        space_id: '101',
      }),
    ).resolves.toMatchObject({
      members: [
        {
          user_id: '9',
          email: 'owner@example.test',
          role_type: 2,
        },
      ],
    });
    expect(fetchMock).toHaveBeenCalledWith('/api/admin/workspaces/members', {
      body: JSON.stringify({
        space_id: '101',
      }),
      credentials: 'include',
      headers: {
        'content-type': 'application/json',
      },
      method: 'POST',
    });
  });

  it('posts admin user workspace lookup', async () => {
    const fetchMock = vi.fn().mockResolvedValue({
      ok: true,
      status: 200,
      json: async () => ({
        spaces: [
          {
            id: '101',
            name: '畅享 AI',
            owner_name: 'Owner',
            role_type: 2,
            total_member_num: 3,
          },
        ],
      }),
    });
    globalThis.fetch = fetchMock as never;

    await expect(
      listAdminUserSpaces({
        user_id: '9',
      }),
    ).resolves.toMatchObject({
      spaces: [
        {
          id: '101',
          name: '畅享 AI',
          role_type: 2,
        },
      ],
    });
    expect(fetchMock).toHaveBeenCalledWith('/api/admin/users/spaces', {
      body: JSON.stringify({
        user_id: '9',
      }),
      credentials: 'include',
      headers: {
        'content-type': 'application/json',
      },
      method: 'POST',
    });
  });

  it('gets admin basic config', async () => {
    const fetchMock = vi.fn().mockResolvedValue({
      ok: true,
      status: 200,
      json: async () => ({
        configuration: {
          admin_emails: 'owner@example.test',
          disable_user_registration: true,
          server_host: 'http://localhost:8888',
        },
      }),
    });
    globalThis.fetch = fetchMock as never;

    await expect(getAdminBasicConfig()).resolves.toMatchObject({
      configuration: {
        admin_emails: 'owner@example.test',
        disable_user_registration: true,
      },
    });
    expect(fetchMock).toHaveBeenCalledWith('/api/admin/config/basic/get', {
      credentials: 'include',
    });
  });

  it('gets admin model list config', async () => {
    const fetchMock = vi.fn().mockResolvedValue({
      ok: true,
      status: 200,
      json: async () => ({
        provider_model_list: [
          {
            provider: {
              name: {
                zh_cn: 'OpenAI 模型',
                en_us: 'OpenAI Model',
              },
            },
            model_list: [
              {
                id: 1,
                display_info: {
                  name: 'gpt-4o',
                },
              },
            ],
          },
        ],
      }),
    });
    globalThis.fetch = fetchMock as never;

    await expect(getAdminModelList()).resolves.toMatchObject({
      provider_model_list: [
        {
          provider: {
            name: {
              zh_cn: 'OpenAI 模型',
            },
          },
          model_list: [
            {
              display_info: {
                name: 'gpt-4o',
              },
            },
          ],
        },
      ],
    });
    expect(fetchMock).toHaveBeenCalledWith('/api/admin/config/model/list', {
      credentials: 'include',
    });
  });

  it('creates admin model config', async () => {
    const fetchMock = vi.fn().mockResolvedValue({
      ok: true,
      status: 200,
      json: async () => ({
        id: 12,
      }),
    });
    globalThis.fetch = fetchMock as never;

    await expect(
      createAdminModel({
        model_class: 1,
        model_name: 'GPT 4o',
        enable_base64_url: true,
        connection: {
          base_conn_info: {
            api_key: 'sk-test',
            base_url: 'https://api.openai.com/v1',
            model: 'gpt-4o',
          },
        },
      }),
    ).resolves.toMatchObject({
      id: 12,
    });
    expect(fetchMock).toHaveBeenCalledWith('/api/admin/config/model/create', {
      body: JSON.stringify({
        model_class: 1,
        model_name: 'GPT 4o',
        enable_base64_url: true,
        connection: {
          base_conn_info: {
            api_key: 'sk-test',
            base_url: 'https://api.openai.com/v1',
            model: 'gpt-4o',
          },
        },
      }),
      credentials: 'include',
      headers: {
        'content-type': 'application/json',
      },
      method: 'POST',
    });
  });

  it('deletes admin model config', async () => {
    const fetchMock = vi.fn().mockResolvedValue({
      ok: true,
      status: 200,
      json: async () => ({}),
    });
    globalThis.fetch = fetchMock as never;

    await expect(deleteAdminModel({ id: 12 })).resolves.toEqual({});
    expect(fetchMock).toHaveBeenCalledWith('/api/admin/config/model/delete', {
      body: JSON.stringify({
        id: '12',
      }),
      credentials: 'include',
      headers: {
        'content-type': 'application/json',
      },
      method: 'POST',
    });
  });

  it('gets admin knowledge config', async () => {
    const fetchMock = vi.fn().mockResolvedValue({
      ok: true,
      status: 200,
      json: async () => ({
        knowledge_config: {
          builtin_model_id: 42,
          embedding_config: {
            type: 1,
          },
        },
      }),
    });
    globalThis.fetch = fetchMock as never;

    await expect(getAdminKnowledgeConfig()).resolves.toMatchObject({
      knowledge_config: {
        builtin_model_id: 42,
      },
    });
    expect(fetchMock).toHaveBeenCalledWith('/api/admin/config/knowledge/get', {
      credentials: 'include',
    });
  });
});
