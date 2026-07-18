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
  space_type?: number;
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

export interface AdminCreateUserPayload {
  email: string;
  password: string;
  name?: string;
  user_unique_name?: string;
  locale?: string;
}

export interface AdminUpdateUserPayload {
  user_id: string;
  name?: string;
  user_unique_name?: string;
  locale?: string;
}

export interface AdminResetUserPasswordPayload {
  user_id: string;
  password: string;
}

export interface AdminUserMutationResponse {
  user?: AdminUser;
  code?: number;
  msg?: string;
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
  allow_registration_email?: string;
  code_runner_type?: number;
  plugin_configuration?: unknown;
  sandbox_config?: unknown;
  server_host?: string;
}

export type AdminBasicConfigPatch = Pick<
  AdminBasicConfig,
  | 'admin_emails'
  | 'disable_user_registration'
  | 'allow_registration_email'
  | 'plugin_configuration'
  | 'server_host'
>;

export interface AdminBasicConfigResponse {
  configuration: AdminBasicConfig;
  revision: string;
}

export interface AdminBasicConfigSaveResponse {
  revision: string;
}

export class AdminAPIError extends Error {
  readonly status: number;
  readonly errorCode?: string;

  constructor(status: number, message: string, errorCode?: string) {
    super(message);
    this.name = 'AdminAPIError';
    this.status = status;
    this.errorCode = errorCode;
  }
}

const throwAdminAPIError = async (response: Response): Promise<never> => {
  const payload = (await response.json().catch(() => ({}))) as {
    error_code?: string;
    msg?: string;
  };
  throw new AdminAPIError(
    response.status,
    payload.msg || `request failed: ${response.status}`,
    payload.error_code,
  );
};

export const isAdminBasicConfigConflict = (error: unknown): boolean =>
  error instanceof AdminAPIError &&
  error.status === 409 &&
  error.errorCode === 'BASE_CONFIG_VERSION_CONFLICT';

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
    await throwAdminAPIError(response);
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
    await throwAdminAPIError(response);
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

export const createAdminUser = (payload: AdminCreateUserPayload) =>
  postJSON<AdminUserMutationResponse>('/api/admin/users/create', payload);

export const updateAdminUser = (payload: AdminUpdateUserPayload) =>
  postJSON<AdminUserMutationResponse>('/api/admin/users/update', payload);

export const resetAdminUserPassword = (
  payload: AdminResetUserPasswordPayload,
) =>
  postJSON<AdminUserMutationResponse>(
    '/api/admin/users/password/reset',
    payload,
  );

export const listAdminWorkspaceMembers = (params: { space_id: string }) =>
  postJSON<{ members: AdminWorkspaceMember[] }>(
    '/api/admin/workspaces/members',
    params,
  );

export const listAdminUserSpaces = (params: { user_id: string }) =>
  postJSON<{ spaces: AdminUserSpace[] }>('/api/admin/users/spaces', params);

export const getAdminBasicConfig =
  async (): Promise<AdminBasicConfigResponse> => {
    const response = await fetch('/api/admin/config/basic/get', {
      credentials: 'include',
    });

    if (!response.ok) {
      await throwAdminAPIError(response);
    }

    const payload =
      (await response.json()) as Partial<AdminBasicConfigResponse>;
    if (!payload.configuration || !payload.revision) {
      throw new AdminAPIError(
        500,
        'basic configuration response is incomplete',
      );
    }
    return payload as AdminBasicConfigResponse;
  };

export const saveAdminBasicConfig = (
  configuration: AdminBasicConfigPatch,
  expectedRevision: string,
) =>
  postJSON<AdminBasicConfigSaveResponse>('/api/admin/config/basic/save', {
    configuration,
    expected_revision: expectedRevision,
  });

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
