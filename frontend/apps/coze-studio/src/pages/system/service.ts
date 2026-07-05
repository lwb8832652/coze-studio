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

export interface SystemAdminStatus {
  is_admin: boolean;
}

interface SystemAdminStatusResponse {
  data?: Partial<SystemAdminStatus>;
}

export interface AdminWorkspace {
  id: string;
  name: string;
  description?: string;
  owner_user_id: string;
  owner_name?: string;
  total_member_num?: number;
  created_at?: number;
}

export interface AdminUser {
  user_id: string;
  name: string;
  email?: string;
  user_unique_name?: string;
  avatar_url?: string;
  created_at?: number;
}

export interface AdminWorkspaceMember {
  user_id: string;
  name: string;
  email?: string;
  user_unique_name?: string;
  avatar_url?: string;
  role_type?: number;
  joined_at?: number;
}

export interface AdminUserSpace {
  id: string;
  name: string;
  description?: string;
  space_type?: number;
  owner_user_id: string;
  owner_name?: string;
  role_type?: number;
  total_member_num?: number;
  created_at?: number;
}

export interface AdminBasicConfig {
  admin_emails?: string;
  disable_user_registration?: boolean;
  server_host?: string;
}

export interface AdminI18nText {
  zh_cn?: string;
  en_us?: string;
}

export interface AdminModelProviderInfo {
  name?: AdminI18nText;
  description?: AdminI18nText;
  model_class?: number;
}

export interface AdminBaseConnectionInfo {
  api_key?: string;
  base_url?: string;
  model?: string;
  thinking_type?: number;
}

export interface AdminModelConnection {
  base_conn_info?: AdminBaseConnectionInfo;
  ark?: unknown;
  openai?: unknown;
  deepseek?: unknown;
  gemini?: unknown;
  qwen?: unknown;
  ollama?: unknown;
  claude?: unknown;
}

export interface AdminModelInfo {
  id?: number | string;
  display_info?: {
    name?: string;
  };
  connection?: AdminModelConnection;
  enable_base64_url?: boolean;
  status?: number;
}

export interface AdminProviderModelListItem {
  provider?: AdminModelProviderInfo;
  model_list?: AdminModelInfo[];
}

export interface AdminModelListResponse {
  provider_model_list?: AdminProviderModelListItem[];
}

export interface AdminCreateModelPayload {
  model_class: number;
  model_name: string;
  connection: AdminModelConnection;
  enable_base64_url?: boolean;
}

export interface AdminCreateModelResponse {
  id?: number | string;
}

export interface AdminKnowledgeConfig {
  builtin_model_id?: number | string;
  embedding_config?: unknown;
  rerank_config?: unknown;
  ocr_config?: unknown;
  parser_config?: unknown;
}

export interface AdminKnowledgeConfigResponse {
  knowledge_config?: AdminKnowledgeConfig;
}

export const getSystemAdminStatus = async (): Promise<SystemAdminStatus> => {
  const response = await fetch('/api/admin/auth/status', {
    credentials: 'include',
  });

  if (response.status === 401 || response.status === 403) {
    return {
      is_admin: false,
    };
  }

  if (!response.ok) {
    throw new Error(`request failed: ${response.status}`);
  }

  const payload = (await response.json()) as SystemAdminStatusResponse;
  return {
    is_admin: Boolean(payload.data?.is_admin),
  };
};

const postJSON = async <T>(url: string, body: unknown): Promise<T> => {
  const response = await fetch(url, {
    body: JSON.stringify(body),
    credentials: 'include',
    headers: {
      'content-type': 'application/json',
    },
    method: 'POST',
  });

  if (!response.ok) {
    throw new Error(`request failed: ${response.status}`);
  }

  return response.json() as Promise<T>;
};

export const listAdminWorkspaces = (params: {
  keyword?: string;
  page?: number;
  size?: number;
}) =>
  postJSON<{ workspaces: AdminWorkspace[]; total: number }>(
    '/api/admin/workspaces/list',
    params,
  );

export const listAdminUsers = (params: {
  keyword?: string;
  page?: number;
  size?: number;
}) =>
  postJSON<{ users: AdminUser[]; total: number }>(
    '/api/admin/users/list',
    params,
  );

export const listAdminWorkspaceMembers = (params: { space_id: string }) =>
  postJSON<{ members: AdminWorkspaceMember[] }>(
    '/api/admin/workspaces/members',
    params,
  );

export const listAdminUserSpaces = (params: { user_id: string }) =>
  postJSON<{ spaces: AdminUserSpace[] }>('/api/admin/users/spaces', params);

export const getAdminBasicConfig = async (): Promise<{
  configuration?: AdminBasicConfig;
}> => {
  const response = await fetch('/api/admin/config/basic/get', {
    credentials: 'include',
  });

  if (!response.ok) {
    throw new Error(`request failed: ${response.status}`);
  }

  return response.json() as Promise<{
    configuration?: AdminBasicConfig;
  }>;
};

export const getAdminModelList = async (): Promise<AdminModelListResponse> => {
  const response = await fetch('/api/admin/config/model/list', {
    credentials: 'include',
  });

  if (!response.ok) {
    throw new Error(`request failed: ${response.status}`);
  }

  return response.json() as Promise<AdminModelListResponse>;
};

export const createAdminModel = (payload: AdminCreateModelPayload) =>
  postJSON<AdminCreateModelResponse>('/api/admin/config/model/create', payload);

export const deleteAdminModel = (params: { id: number | string }) =>
  postJSON<Record<string, never>>('/api/admin/config/model/delete', {
    id: String(params.id),
  });

export const getAdminKnowledgeConfig =
  async (): Promise<AdminKnowledgeConfigResponse> => {
    const response = await fetch('/api/admin/config/knowledge/get', {
      credentials: 'include',
    });

    if (!response.ok) {
      throw new Error(`request failed: ${response.status}`);
    }

    return response.json() as Promise<AdminKnowledgeConfigResponse>;
  };
