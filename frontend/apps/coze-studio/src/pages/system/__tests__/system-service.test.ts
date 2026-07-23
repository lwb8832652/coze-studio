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

/* eslint-disable @typescript-eslint/require-await -- Async mocks mirror production contracts. */

import { afterEach, describe, expect, it, vi } from 'vitest';

import {
  createAdminModel,
  createAdminManagedModel,
  createAdminUser,
  deleteAdminModel,
  getAdminBasicConfig,
  getAdminKnowledgeConfig,
  getAdminModelList,
  listAdminManagedModels,
  listAdminModelProviders,
  getSystemAdminStatus,
  isAdminBasicConfigConflict,
  listAdminUserSpaces,
  listAdminWorkspaceMembers,
  listAdminUsers,
  listAdminWorkspaces,
  resetAdminUserPassword,
  saveAdminBasicConfig,
  updateAdminUser,
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

  it('creates admin user', async () => {
    const fetchMock = vi.fn().mockResolvedValue({
      ok: true,
      status: 200,
      json: async () => ({
        user: {
          user_id: '99',
          email: 'new@example.test',
        },
      }),
    });
    globalThis.fetch = fetchMock as never;

    await expect(
      createAdminUser({
        email: 'new@example.test',
        password: 'secret1',
        name: 'New User',
        user_unique_name: 'new-user',
        locale: 'zh-CN',
      }),
    ).resolves.toMatchObject({
      user: {
        user_id: '99',
      },
    });
    expect(fetchMock).toHaveBeenCalledWith('/api/admin/users/create', {
      body: JSON.stringify({
        email: 'new@example.test',
        password: 'secret1',
        name: 'New User',
        user_unique_name: 'new-user',
        locale: 'zh-CN',
      }),
      credentials: 'include',
      headers: {
        'content-type': 'application/json',
      },
      method: 'POST',
    });
  });

  it('updates admin user', async () => {
    const fetchMock = vi.fn().mockResolvedValue({
      ok: true,
      status: 200,
      json: async () => ({
        msg: 'success',
      }),
    });
    globalThis.fetch = fetchMock as never;

    await expect(
      updateAdminUser({
        user_id: '9',
        name: 'Owner Edited',
        user_unique_name: 'owner-edited',
        locale: 'zh-CN',
      }),
    ).resolves.toMatchObject({
      msg: 'success',
    });
    expect(fetchMock).toHaveBeenCalledWith('/api/admin/users/update', {
      body: JSON.stringify({
        user_id: '9',
        name: 'Owner Edited',
        user_unique_name: 'owner-edited',
        locale: 'zh-CN',
      }),
      credentials: 'include',
      headers: {
        'content-type': 'application/json',
      },
      method: 'POST',
    });
  });

  it('resets admin user password', async () => {
    const fetchMock = vi.fn().mockResolvedValue({
      ok: true,
      status: 200,
      json: async () => ({
        msg: 'success',
      }),
    });
    globalThis.fetch = fetchMock as never;

    await expect(
      resetAdminUserPassword({
        user_id: '9',
        password: 'secret2',
      }),
    ).resolves.toMatchObject({
      msg: 'success',
    });
    expect(fetchMock).toHaveBeenCalledWith('/api/admin/users/password/reset', {
      body: JSON.stringify({
        user_id: '9',
        password: 'secret2',
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
        revision: 'rev-7',
        configuration: {
          admin_emails: 'owner@example.test',
          disable_user_registration: true,
          server_host: 'http://localhost:8888',
        },
      }),
    });
    globalThis.fetch = fetchMock as never;

    await expect(getAdminBasicConfig()).resolves.toMatchObject({
      revision: 'rev-7',
      configuration: {
        admin_emails: 'owner@example.test',
        disable_user_registration: true,
      },
    });
    expect(fetchMock).toHaveBeenCalledWith('/api/admin/config/basic/get', {
      credentials: 'include',
    });
  });

  it('saves admin basic config', async () => {
    const fetchMock = vi.fn().mockResolvedValue({
      ok: true,
      status: 200,
      json: async () => ({ revision: 'rev-8' }),
    });
    globalThis.fetch = fetchMock as never;

    await expect(
      saveAdminBasicConfig(
        {
          server_host: 'https://agent.example.test',
        },
        'rev-7',
      ),
    ).resolves.toEqual({ revision: 'rev-8' });
    expect(fetchMock).toHaveBeenCalledWith('/api/admin/config/basic/save', {
      body: JSON.stringify({
        configuration: {
          server_host: 'https://agent.example.test',
        },
        expected_revision: 'rev-7',
      }),
      credentials: 'include',
      headers: {
        'content-type': 'application/json',
      },
      method: 'POST',
    });
  });

  it('exposes a stable basic config conflict error', async () => {
    globalThis.fetch = vi.fn().mockResolvedValue({
      ok: false,
      status: 409,
      json: async () => ({
        error_code: 'BASE_CONFIG_VERSION_CONFLICT',
        msg: 'base configuration revision is stale',
      }),
    }) as never;

    const error = await saveAdminBasicConfig(
      { admin_emails: 'owner@example.test' },
      'rev-stale',
    ).catch(reason => reason);

    expect(isAdminBasicConfigConflict(error)).toBe(true);
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
        preview: false,
      }),
      credentials: 'include',
      headers: {
        'content-type': 'application/json',
      },
      method: 'POST',
    });
  });

  it('queries and creates managed system models without echoing credentials', async () => {
    const fetchMock = vi
      .fn()
      .mockResolvedValueOnce({
        ok: true,
        status: 200,
        json: async () => ({
          models: [{ id: '12', name: 'DeepSeek V4 Pro' }],
          total: 1,
        }),
      })
      .mockResolvedValueOnce({
        ok: true,
        status: 200,
        json: async () => ({
          providers: [{ provider_key: 'deepseek', model_class: 3 }],
        }),
      })
      .mockResolvedValueOnce({
        ok: true,
        status: 200,
        json: async () => ({ id: '13' }),
      });
    globalThis.fetch = fetchMock as never;

    await expect(
      listAdminManagedModels({ enabled: true, capability_type: 'text' }),
    ).resolves.toMatchObject({ total: 1 });
    await expect(listAdminModelProviders()).resolves.toMatchObject({
      providers: [{ provider_key: 'deepseek' }],
    });
    await expect(
      createAdminManagedModel({
        model_class: 3,
        management: {
          access_mode: 1,
          capability_types: ['text'],
          enabled: true,
          endpoints: [
            {
              api_key: 'write-only-secret',
              base_url: 'https://api.example.test/v1',
              enabled: true,
              sort_order: 0,
              weight: 1,
            },
          ],
          function_call_mode: 'native',
          max_context_tokens: 128000,
          max_output_tokens: 8192,
          model_identifier: 'deepseek-v4-pro',
          name: 'DeepSeek V4 Pro',
          protocol: 'openai-compatible',
          provider_key: 'deepseek',
          reasoning_mode: 'enabled',
          routing_strategy: 1,
          usage_scenarios: ['chat'],
        },
      }),
    ).resolves.toEqual({ id: '13' });

    expect(fetchMock.mock.calls[0]?.[0]).toContain(
      '/api/admin/config/model/manage/list?',
    );
    expect(fetchMock.mock.calls[0]?.[0]).toContain('capability_type=text');
    expect(fetchMock.mock.calls[0]?.[0]).toContain('enabled=true');
    expect(fetchMock.mock.calls[1]?.[0]).toBe(
      '/api/admin/config/model/providers',
    );
    expect(fetchMock.mock.calls[2]?.[0]).toBe('/api/admin/config/model/create');
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
