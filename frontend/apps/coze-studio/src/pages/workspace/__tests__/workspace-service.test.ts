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
  addWorkspaceMembers,
  deleteWorkspace,
  getWorkspaceDetail,
  listWorkspaceMembers,
  removeWorkspaceMember,
  searchWorkspaceUsers,
  transferWorkspace,
  updateWorkspace,
  updateWorkspaceMemberRole,
} from '../service';

describe('workspace service', () => {
  const previousFetch = globalThis.fetch;

  afterEach(() => {
    globalThis.fetch = previousFetch;
    vi.restoreAllMocks();
  });

  it('loads workspace detail with encoded space id', async () => {
    const fetchMock = vi.fn().mockResolvedValue({
      ok: true,
      status: 200,
      json: async () => ({
        data: {
          id: 'space/1',
          name: '畅享 AI',
          current_user_role: 1,
          total_member_num: 2,
        },
      }),
    });
    globalThis.fetch = fetchMock as never;

    await expect(getWorkspaceDetail('space/1')).resolves.toMatchObject({
      data: {
        id: 'space/1',
        name: '畅享 AI',
        current_user_role: 1,
        total_member_num: 2,
      },
    });
    expect(fetchMock).toHaveBeenCalledWith(
      '/api/workspace/space/detail?space_id=space%2F1',
      {
        credentials: 'include',
      },
    );
  });

  it('posts workspace member filter parameters', async () => {
    const fetchMock = vi.fn().mockResolvedValue({
      ok: true,
      status: 200,
      json: async () => ({
        members: [
          {
            user_id: '9',
            name: 'Owner',
            role_type: 1,
            joined_at: 1717000000,
          },
        ],
      }),
    });
    globalThis.fetch = fetchMock as never;

    await expect(
      listWorkspaceMembers({
        space_id: '101',
        keyword: 'owner',
        role_type: 1,
      }),
    ).resolves.toMatchObject({
      members: [
        {
          user_id: '9',
          name: 'Owner',
          role_type: 1,
        },
      ],
    });
    expect(fetchMock).toHaveBeenCalledWith('/api/workspace/space/members', {
      body: JSON.stringify({
        space_id: '101',
        keyword: 'owner',
        role_type: 1,
      }),
      credentials: 'include',
      headers: {
        'content-type': 'application/json',
      },
      method: 'POST',
    });
  });

  it('posts workspace user search parameters', async () => {
    const fetchMock = vi.fn().mockResolvedValue({
      ok: true,
      status: 200,
      json: async () => ({
        users: [
          {
            user_id: '11',
            name: 'New User',
            role_type: 0,
          },
        ],
        total: 1,
      }),
    });
    globalThis.fetch = fetchMock as never;

    await expect(
      searchWorkspaceUsers({
        space_id: '101',
        keyword: 'new',
        limit: 20,
      }),
    ).resolves.toMatchObject({
      users: [
        {
          user_id: '11',
          name: 'New User',
        },
      ],
    });
    expect(fetchMock).toHaveBeenCalledWith('/api/workspace/space/users/search', {
      body: JSON.stringify({
        space_id: '101',
        keyword: 'new',
        limit: 20,
      }),
      credentials: 'include',
      headers: {
        'content-type': 'application/json',
      },
      method: 'POST',
    });
  });

  it('posts workspace mutation requests', async () => {
    const fetchMock = vi.fn().mockResolvedValue({
      ok: true,
      status: 200,
      json: async () => ({
        code: 0,
        msg: 'success',
      }),
    });
    globalThis.fetch = fetchMock as never;

    await updateWorkspace({
      space_id: '101',
      name: '畅享 AI',
      description: '团队空间',
      allow_develop: false,
      receive_publish: true,
    });
    await addWorkspaceMembers({
      space_id: '101',
      members: [{ user_id: '11', role_type: 3 }],
    });
    await updateWorkspaceMemberRole({
      space_id: '101',
      user_id: '11',
      role_type: 2,
    });
    await removeWorkspaceMember({
      space_id: '101',
      user_id: '11',
    });
    await transferWorkspace({
      space_id: '101',
      target_user_id: '11',
    });
    await deleteWorkspace({
      space_id: '101',
    });

    expect(fetchMock).toHaveBeenNthCalledWith(1, '/api/workspace/space/update', {
      body: JSON.stringify({
        space_id: '101',
        name: '畅享 AI',
        description: '团队空间',
        allow_develop: false,
        receive_publish: true,
      }),
      credentials: 'include',
      headers: {
        'content-type': 'application/json',
      },
      method: 'POST',
    });
    expect(fetchMock).toHaveBeenNthCalledWith(
      2,
      '/api/workspace/space/members/add',
      expect.objectContaining({ method: 'POST' }),
    );
    expect(fetchMock).toHaveBeenNthCalledWith(
      3,
      '/api/workspace/space/member/role',
      expect.objectContaining({ method: 'POST' }),
    );
    expect(fetchMock).toHaveBeenNthCalledWith(
      4,
      '/api/workspace/space/member/remove',
      expect.objectContaining({ method: 'POST' }),
    );
    expect(fetchMock).toHaveBeenNthCalledWith(
      5,
      '/api/workspace/space/transfer',
      expect.objectContaining({ method: 'POST' }),
    );
    expect(fetchMock).toHaveBeenNthCalledWith(
      6,
      '/api/workspace/space/delete',
      expect.objectContaining({ method: 'POST' }),
    );
  });

  it('throws when workspace request fails', async () => {
    globalThis.fetch = vi.fn().mockResolvedValue({
      ok: false,
      status: 500,
      json: async () => ({}),
    }) as never;

    await expect(getWorkspaceDetail('101')).rejects.toThrow(
      'request failed: 500',
    );
  });
});
