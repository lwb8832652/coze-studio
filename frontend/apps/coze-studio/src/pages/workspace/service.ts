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

export interface WorkspaceDetail {
  id: string;
  name: string;
  description?: string;
  icon_url?: string;
  space_type?: number;
  owner_user_id: string;
  current_user_role: number;
  total_member_num?: number;
  allow_develop?: boolean;
  receive_publish?: boolean;
}

export interface WorkspaceMember {
  user_id: string;
  name: string;
  user_unique_name?: string;
  email?: string;
  avatar_url?: string;
  role_type: number;
  joined_at: number;
}

export interface WorkspaceUserCandidate {
  user_id: string;
  name: string;
  user_unique_name?: string;
  email?: string;
  avatar_url?: string;
  role_type?: number;
}

export interface WorkspaceDetailResponse {
  data: WorkspaceDetail;
  code?: number;
  msg?: string;
}

export interface WorkspaceMembersResponse {
  members: WorkspaceMember[];
  code?: number;
  msg?: string;
}

export interface WorkspaceUsersResponse {
  users: WorkspaceUserCandidate[];
  total: number;
  code?: number;
  msg?: string;
}

export interface WorkspaceMutationResponse {
  code?: number;
  msg?: string;
}

const requestJSON = async <T>(url: string, init?: RequestInit): Promise<T> => {
  const response = await fetch(url, {
    credentials: 'include',
    ...init,
  });

  if (!response.ok) {
    throw new Error(`request failed: ${response.status}`);
  }

  return response.json() as Promise<T>;
};

export const getWorkspaceDetail = (
  spaceId: string,
): Promise<WorkspaceDetailResponse> =>
  requestJSON(
    `/api/workspace/space/detail?space_id=${encodeURIComponent(spaceId)}`,
  );

export const listWorkspaceMembers = (params: {
  space_id: string;
  keyword?: string;
  role_type?: number;
}): Promise<WorkspaceMembersResponse> =>
  requestJSON('/api/workspace/space/members', {
    body: JSON.stringify(params),
    headers: {
      'content-type': 'application/json',
    },
    method: 'POST',
  });

export const searchWorkspaceUsers = (params: {
  space_id: string;
  keyword?: string;
  limit?: number;
}): Promise<WorkspaceUsersResponse> =>
  requestJSON('/api/workspace/space/users/search', {
    body: JSON.stringify(params),
    headers: {
      'content-type': 'application/json',
    },
    method: 'POST',
  });

export const updateWorkspace = (params: {
  space_id: string;
  name?: string;
  description?: string;
  icon_uri?: string;
  allow_develop?: boolean;
  receive_publish?: boolean;
}): Promise<WorkspaceMutationResponse> =>
  requestJSON('/api/workspace/space/update', {
    body: JSON.stringify(params),
    headers: {
      'content-type': 'application/json',
    },
    method: 'POST',
  });

export const addWorkspaceMembers = (params: {
  space_id: string;
  members: Array<{
    user_id: string;
    role_type: number;
  }>;
}): Promise<WorkspaceMutationResponse> =>
  requestJSON('/api/workspace/space/members/add', {
    body: JSON.stringify(params),
    headers: {
      'content-type': 'application/json',
    },
    method: 'POST',
  });

export const updateWorkspaceMemberRole = (params: {
  space_id: string;
  user_id: string;
  role_type: number;
}): Promise<WorkspaceMutationResponse> =>
  requestJSON('/api/workspace/space/member/role', {
    body: JSON.stringify(params),
    headers: {
      'content-type': 'application/json',
    },
    method: 'POST',
  });

export const removeWorkspaceMember = (params: {
  space_id: string;
  user_id: string;
}): Promise<WorkspaceMutationResponse> =>
  requestJSON('/api/workspace/space/member/remove', {
    body: JSON.stringify(params),
    headers: {
      'content-type': 'application/json',
    },
    method: 'POST',
  });

export const transferWorkspace = (params: {
  space_id: string;
  target_user_id: string;
}): Promise<WorkspaceMutationResponse> =>
  requestJSON('/api/workspace/space/transfer', {
    body: JSON.stringify(params),
    headers: {
      'content-type': 'application/json',
    },
    method: 'POST',
  });

export const deleteWorkspace = (params: {
  space_id: string;
}): Promise<WorkspaceMutationResponse> =>
  requestJSON('/api/workspace/space/delete', {
    body: JSON.stringify(params),
    headers: {
      'content-type': 'application/json',
    },
    method: 'POST',
  });
