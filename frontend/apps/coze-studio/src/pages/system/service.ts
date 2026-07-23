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

/* eslint-disable @typescript-eslint/naming-convention, max-lines -- System administration DTOs intentionally preserve server wire names. */

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
  site_name?: string;
  site_description?: string;
  site_logo_uri?: string;
  favicon_uri?: string;
}

export type AdminBasicConfigPatch = Pick<
  AdminBasicConfig,
  | 'admin_emails'
  | 'disable_user_registration'
  | 'allow_registration_email'
  | 'plugin_configuration'
  | 'server_host'
  | 'site_name'
  | 'site_description'
  | 'site_logo_uri'
  | 'favicon_uri'
>;

export interface AdminBasicConfigResponse {
  configuration: AdminBasicConfig;
  revision: string;
}

export interface AdminBasicConfigSaveResponse {
  revision: string;
}

export type AdminSiteAssetKind = 'logo' | 'favicon';

export interface AdminSiteAssetUploadResponse {
  uri: string;
  url: string;
  width: number;
  height: number;
  mime_type: string;
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

export interface BillingEnvelope<T> {
  code: number;
  msg: string;
  data: T;
}
export interface BillingConfig {
  credit_name: string;
  display_scale: number;
  allow_negative: boolean;
  settlement_enabled: boolean;
  payment_enabled: boolean;
  default_currency: string;
  version: number;
}
export interface BillingOverview {
  Accounts: number;
  Plans: number;
  Packages: number;
  PendingOrders: number;
  UsageRecords: number;
}
export interface BillingPlan {
  id: string;
  key: string;
  name: string;
  description: string;
  status: string;
  sort_order: number;
  current_version: number;
  cycle: string;
  price_micros: number;
  currency: string;
  credit_grant_micros: number;
  features_json: string;
  effective_at: string;
}
export interface BillingPackage {
  ID: string;
  Key: string;
  Name: string;
  Description: string;
  Status: string;
  PriceMicros: number;
  Currency: string;
  CreditMicros: number;
  ValidityDays: number;
}
export interface BillingAccount {
  id: string;
  subject_type: string;
  subject_id: string;
  available_micros: number;
  reserved_micros: number;
  version: number;
  updated_at: string;
}
export interface BillingLedger {
  id: string;
  account_id: string;
  Direction: string;
  EntryType: string;
  AmountMicros: number;
  AvailableAfterMicros: number;
  ReservedAfterMicros: number;
  BusinessNo: string;
  CreatedAt: string;
}
export interface BillingOrder {
  id: string;
  OrderNo: string;
  UserID: string;
  OrderType: string;
  Status: string;
  PaymentStatus: string;
  FulfillmentStatus: string;
  TotalMicros: number;
  Currency: string;
  CreatedAt: string;
}
export interface BillingModelPrice {
  id: string;
  Provider: string;
  ModelID: string;
  Version: number;
  Currency: string;
  InputPrice: number;
  OutputPrice: number;
  CacheWritePrice: number;
  CacheHitPrice: number;
  Status: string;
  EffectiveAt: string;
}
export interface BillingUsageMonitor {
  provider: string;
  model_id: string;
  calls: number;
  input_tokens: number;
  output_tokens: number;
  cache_write_tokens: number;
  cache_hit_tokens: number;
  charge_micros: number;
  last_used_at: string;
}
export interface BillingMaintenanceResult {
  released_reservations: number;
  expired_subscriptions: number;
  closed_orders: number;
  reconciliation?: {
    run_id: string;
    checked_count: number;
    mismatch_count: number;
    mismatches: Array<{
      account_id: string;
      available_micros: number;
      expected_available: number;
      reserved_micros: number;
      expected_reserved: number;
    }>;
  };
  errors: string[];
}

const billingRequest = async <T>(
  path: string,
  init?: RequestInit,
): Promise<T> => {
  const response = await fetch(`/api/admin/billing${path}`, {
    credentials: 'include',
    ...init,
    headers: { 'Content-Type': 'application/json', ...(init?.headers ?? {}) },
  });
  if (!response.ok) {
    return throwAdminAPIError(response);
  }
  const payload = (await response.json()) as BillingEnvelope<T>;
  if (payload.code !== 0) {
    throw new AdminAPIError(response.status, payload.msg || '请求失败');
  }
  return payload.data;
};

export const getBillingOverview = () =>
  billingRequest<BillingOverview>('/overview');
export const getBillingConfig = () => billingRequest<BillingConfig>('/config');
export const saveBillingConfig = (value: BillingConfig) =>
  billingRequest<BillingConfig>('/config', {
    method: 'PUT',
    body: JSON.stringify(value),
  });
export const listBillingPlans = () => billingRequest<BillingPlan[]>('/plans');
export const createBillingPlan = (value: Record<string, unknown>) =>
  billingRequest<BillingPlan>('/plans', {
    method: 'POST',
    body: JSON.stringify(value),
  });
export const listBillingPackages = () =>
  billingRequest<BillingPackage[]>('/credit-packages');
export const createBillingPackage = (value: Record<string, unknown>) =>
  billingRequest<BillingPackage>('/credit-packages', {
    method: 'POST',
    body: JSON.stringify(value),
  });
export const listBillingAccounts = () =>
  billingRequest<BillingAccount[]>('/accounts');
export const listBillingLedger = () =>
  billingRequest<BillingLedger[]>('/ledger');
export const listBillingOrders = () =>
  billingRequest<BillingOrder[]>('/orders');
export const listBillingModelPrices = () =>
  billingRequest<BillingModelPrice[]>('/model-prices');
export const createBillingModelPrice = (value: Record<string, unknown>) =>
  billingRequest<BillingModelPrice>('/model-prices', {
    method: 'POST',
    body: JSON.stringify(value),
  });
export const listBillingUsageMonitoring = () =>
  billingRequest<BillingUsageMonitor[]>('/usage-monitoring');
export const adjustBillingCredits = (value: {
  subject_type: string;
  subject_id: string;
  amount_micros: number;
  business_no: string;
  reason: string;
}) =>
  billingRequest<{ balance: BillingAccount; action: string }>('/adjustments', {
    method: 'POST',
    body: JSON.stringify(value),
  });
export const runBillingMaintenance = () =>
  billingRequest<BillingMaintenanceResult>('/maintenance/run', {
    method: 'POST',
    body: '{}',
  });

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

export interface AdminModelProviderOption {
  provider_key: string;
  name: AdminI18nText;
  model_class: number;
  protocol: string;
  supports_custom_base_url: boolean;
  supports_function_call: boolean;
  supports_multimodal: boolean;
  default_base_url?: string;
}

export interface AdminManagedModel {
  id: number | string;
  provider_key: string;
  model_class: number;
  name: string;
  model_identifier: string;
  description?: string;
  capability_types: string[];
  enabled: boolean;
  access_mode: number;
  creator_id: number | string;
  creator_name?: string;
  updated_at_ms: number;
  sort_order: number;
}

export interface AdminModelEndpointInput {
  id?: number | string;
  base_url: string;
  api_key?: string;
  clear_api_key?: boolean;
  weight: number;
  enabled: boolean;
  sort_order: number;
}

export interface AdminModelEndpointView {
  id: number | string;
  base_url: string;
  has_api_key: boolean;
  weight: number;
  enabled: boolean;
  sort_order: number;
}

export interface AdminModelManagementInput {
  provider_key: string;
  name: string;
  model_identifier: string;
  description?: string;
  capability_types: string[];
  reasoning_mode: string;
  max_context_tokens: number;
  max_output_tokens: number;
  function_call_mode: string;
  enabled: boolean;
  usage_scenarios: string[];
  protocol: string;
  routing_strategy: number;
  access_mode: number;
  endpoints: AdminModelEndpointInput[];
  enable_base64_url?: boolean;
}

export interface AdminModelDetail {
  summary: AdminManagedModel;
  reasoning_mode: string;
  max_context_tokens: number;
  max_output_tokens: number;
  function_call_mode: string;
  usage_scenarios: string[];
  protocol: string;
  routing_strategy: number;
  endpoints: AdminModelEndpointView[];
  enable_base64_url: boolean;
}

export interface AdminModelGrantSubject {
  subject_type: number;
  subject_id: number | string;
  name?: string;
  description?: string;
}

export interface AdminModelListFilters {
  keyword?: string;
  provider_key?: string;
  capability_type?: string;
  enabled?: boolean;
  access_mode?: string;
  page?: number;
  page_size?: number;
}

export interface AdminModelEndpointTestPayload {
  model_id?: number | string;
  endpoint: AdminModelEndpointInput;
  provider_key: string;
  model_identifier: string;
  protocol: string;
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

const getJSON = async <T>(url: string): Promise<T> => {
  const response = await fetch(url, {
    credentials: 'include',
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

export const uploadAdminSiteAsset = async (
  kind: AdminSiteAssetKind,
  file: File,
): Promise<AdminSiteAssetUploadResponse> => {
  const body = new FormData();
  body.set('kind', kind);
  body.set('file', file);
  const response = await fetch('/api/admin/config/site/assets', {
    method: 'POST',
    credentials: 'include',
    body,
  });
  if (!response.ok) {
    await throwAdminAPIError(response);
  }
  return response.json() as Promise<AdminSiteAssetUploadResponse>;
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

export const deleteAdminModel = (params: {
  id: number | string;
  preview?: boolean;
}) =>
  postJSON<{
    dependencies?: Array<{ dependency_type: string; count: number }>;
  }>('/api/admin/config/model/delete', {
    id: String(params.id),
    preview: Boolean(params.preview),
  });

export const listAdminModelProviders = () =>
  getJSON<{ providers: AdminModelProviderOption[] }>(
    '/api/admin/config/model/providers',
  );

export const listAdminManagedModels = (filters: AdminModelListFilters = {}) => {
  const query = new URLSearchParams();
  Object.entries(filters).forEach(([key, value]) => {
    if (value !== undefined && value !== '') {
      query.set(key, String(value));
    }
  });
  const suffix = query.size > 0 ? `?${query.toString()}` : '';
  return getJSON<{ models: AdminManagedModel[]; total?: number }>(
    `/api/admin/config/model/manage/list${suffix}`,
  );
};

export const getAdminManagedModelDetail = (id: number | string) =>
  getJSON<{ model: AdminModelDetail }>(
    `/api/admin/config/model/detail?id=${encodeURIComponent(String(id))}`,
  );

export const createAdminManagedModel = (payload: {
  model_class: number;
  management: AdminModelManagementInput;
}) =>
  postJSON<AdminCreateModelResponse>('/api/admin/config/model/create', {
    connection: {
      base_conn_info: {
        api_key: '',
        base_url: payload.management.endpoints[0]?.base_url || '',
        model: payload.management.model_identifier,
      },
    },
    enable_base64_url: Boolean(payload.management.enable_base64_url),
    management: payload.management,
    model_class: payload.model_class,
    model_name: payload.management.name,
  });

export const updateAdminManagedModel = (payload: {
  id: number | string;
  management: AdminModelManagementInput;
}) =>
  postJSON<Record<string, never>>('/api/admin/config/model/update', {
    id: String(payload.id),
    management: payload.management,
  });

export const testAdminManagedModelEndpoint = (
  payload: AdminModelEndpointTestPayload,
) =>
  postJSON<{
    success: boolean;
    latency_ms: number;
    error_code?: string;
    error_message?: string;
  }>('/api/admin/config/model/test', {
    ...payload,
    model_id: payload.model_id ? String(payload.model_id) : undefined,
  });

export const updateAdminManagedModelStatus = (payload: {
  id: number | string;
  enabled: boolean;
}) =>
  postJSON<Record<string, never>>('/api/admin/config/model/status', {
    enabled: payload.enabled,
    id: String(payload.id),
  });

export const sortAdminManagedModels = (
  items: Array<{ id: number | string; sort_order: number }>,
) =>
  postJSON<Record<string, never>>('/api/admin/config/model/sort', {
    items: items.map(item => ({
      id: String(item.id),
      sort_order: item.sort_order,
    })),
  });

export const getAdminManagedModelGrants = (modelID: number | string) =>
  getJSON<{ access_mode: number; grants: AdminModelGrantSubject[] }>(
    `/api/admin/config/model/grants?model_id=${encodeURIComponent(
      String(modelID),
    )}`,
  );

export const saveAdminManagedModelGrants = (payload: {
  model_id: number | string;
  access_mode: number;
  grants: AdminModelGrantSubject[];
}) =>
  postJSON<Record<string, never>>('/api/admin/config/model/grants', {
    access_mode: payload.access_mode,
    grants: payload.grants.map(grant => ({
      ...grant,
      subject_id: String(grant.subject_id),
    })),
    model_id: String(payload.model_id),
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
