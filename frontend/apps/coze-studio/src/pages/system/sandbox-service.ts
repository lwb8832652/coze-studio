/* Copyright 2025 coze-dev Authors */

/* eslint-disable @typescript-eslint/naming-convention -- Legacy Go wire payloads may expose PascalCase policy fields. */
/* eslint-disable no-control-regex, max-lines, max-params -- The centralized API client redacts control characters and preserves endpoint signatures. */

export type SandboxScope = 'agent' | 'mcp_stdio' | 'appdev' | 'plugin';
export type SandboxProviderType = 'remote_http' | 'local_debug';
export type SandboxProviderStatus = 'enabled' | 'disabled';
export type SandboxHealthStatus =
  | 'unknown'
  | 'checking'
  | 'healthy'
  | 'degraded'
  | 'unhealthy';
export type SandboxSecretMode = 'keep' | 'replace' | 'clear';
export type SandboxNodeModulesMode = 'disabled' | 'approved_directory';

export interface SandboxRuntimePolicy {
  timeout_seconds: number;
  memory_limit_mb: number;
  cpu_limit: number;
  max_output_bytes: number;
  max_concurrency: number;
  allow_network: boolean;
  network_allowlist: string[];
  allowed_env_names: string[];
  virtual_read_prefixes: string[];
  virtual_write_prefixes: string[];
  allowed_executables: string[];
  ffi_enabled: boolean;
  node_modules_mode: SandboxNodeModulesMode;
  node_modules_directory_ref: string;
}

export interface SandboxHealthProjection {
  status: SandboxHealthStatus;
  capabilities: SandboxScope[];
  reason_code: string;
  message: string;
  latency_bucket: string;
  checked_at: string;
}

export interface SandboxProvider {
  id: number;
  name: string;
  type: SandboxProviderType;
  endpoint_hint: string;
  credential_configured: boolean;
  credential_fingerprint: string;
  active: boolean;
  needs_rewrap: boolean;
  scopes: SandboxScope[];
  policy: SandboxRuntimePolicy;
  status: SandboxProviderStatus;
  health: SandboxHealthProjection;
  version: number;
  created_at: string;
  updated_at: string;
}

export interface SandboxProviderList {
  items: SandboxProvider[];
  total: number;
}

export interface SandboxProviderDefault {
  scope: SandboxScope;
  provider_id: number;
  version: number;
  updated_at: string;
}

export interface SandboxProviderDefaultProjection {
  scope: SandboxScope;
  configured: boolean;
  provider_id: number | null;
  provider_name: string;
  provider_type: SandboxProviderType | '';
  provider_status: SandboxProviderStatus | '';
  version: number;
  provider_version: number;
  updated_at: string | null;
}

export interface SandboxProviderDefaults {
  items: SandboxProviderDefaultProjection[];
}

export interface SandboxProviderSummary {
  total: number;
  enabled: number;
  unhealthy: number;
}

export interface SandboxCapabilityState {
  available: boolean;
  reason_code: string;
  message: string;
}

export interface SandboxCapabilities {
  control_plane: SandboxCapabilityState;
  local_debug: SandboxCapabilityState;
}

export interface SandboxProviderMutation {
  provider_id: number;
  version: number;
}

export interface SandboxSchedulerWorkload {
  weight: number;
  cpu_limit: number;
  memory_limit_mb: number;
  pid_limit: number;
  queue_timeout_seconds: number;
  idle_ttl_seconds: number;
}

export interface SandboxSchedulerSettings {
  total_weight: number;
  max_outstanding: number;
  global_queue_depth: number;
  per_space_queue_depth: number;
  per_user_queue_depth: number;
  host_memory_reserve_mb: number;
  cancel_grace_seconds: number;
  health_failure_threshold: number;
  health_recovery_threshold: number;
  workloads: Record<SandboxScope, SandboxSchedulerWorkload>;
}

export interface SandboxSchedulerSettingsSnapshot {
  version: number;
  settings: SandboxSchedulerSettings;
}

export interface SandboxSchedulerSettingsUpdate
  extends SandboxSchedulerSettingsSnapshot {
  applied: boolean;
  reason_code?: string;
}

export interface SandboxRuntimeStatus {
  available: boolean;
  desired_config_version: number;
  applied_config_version: number;
  queue_depth: number;
  queue_by_scope?: Partial<Record<SandboxScope, number>>;
  active_slots: number;
  slot_capacity: number;
  queue_high_watermark: number;
  draining_count: number;
  quarantined_count: number;
  idle_containers: number;
  active_containers: number;
  memory_reserve_state?: 'available' | 'below_watermark' | 'unknown';
  reason_code?: string;
}

export interface SandboxAuditEvent {
  id: number;
  provider_id: number;
  actor_user_id: number;
  action: string;
  result: string;
  request_id: string;
  metadata: Record<string, string>;
  created_at: string;
}

export interface SandboxAuditEventList {
  items: SandboxAuditEvent[];
  total: number;
}

export interface SandboxProviderFilters {
  keyword?: string;
  type?: SandboxProviderType | '';
  status?: SandboxProviderStatus | '';
  health?: Exclude<SandboxHealthStatus, 'checking'> | '';
  scope?: SandboxScope | '';
  offset: number;
  limit: number;
}

export interface SandboxAuditFilters {
  action?: string;
  result?: string;
  offset: number;
  limit: number;
}

export interface SandboxCreateProviderPayload {
  name: string;
  type: SandboxProviderType;
  endpoint: string;
  credential: string;
  scopes: SandboxScope[];
  policy: SandboxRuntimePolicy;
}

export interface SandboxSecretMutation {
  mode: SandboxSecretMode;
  value: string;
}

export interface SandboxUpdateProviderPayload {
  expected_version: number;
  name: string;
  scopes: SandboxScope[];
  policy: SandboxRuntimePolicy;
  endpoint?: SandboxSecretMutation;
  credential?: SandboxSecretMutation;
}

type SandboxFieldErrors = Record<string, string>;

interface SandboxEnvelope<T> {
  data?: T;
  code: number;
  error_code?: string;
  field_errors?: SandboxFieldErrors;
  msg: string;
}

interface SandboxRuntimePolicyWire {
  timeout_seconds?: number;
  TimeoutSeconds?: number;
  memory_limit_mb?: number;
  MemoryLimitMB?: number;
  cpu_limit?: number;
  CPULimit?: number;
  max_output_bytes?: number;
  MaxOutputBytes?: number;
  max_concurrency?: number;
  MaxConcurrency?: number;
  allow_network?: boolean;
  AllowNetwork?: boolean;
  network_allowlist?: string[];
  NetworkAllowlist?: string[];
  allowed_env_names?: string[];
  virtual_read_prefixes?: string[];
  virtual_write_prefixes?: string[];
  allowed_executables?: string[];
  ffi_enabled?: boolean;
  node_modules_mode?: SandboxNodeModulesMode;
  node_modules_directory_ref?: string;
}

interface SandboxProviderWire extends Omit<SandboxProvider, 'policy'> {
  policy: SandboxRuntimePolicyWire;
}

interface SandboxProviderListWire {
  items?: SandboxProviderWire[];
  total?: number;
}

interface SandboxAuditEventListWire {
  items?: SandboxAuditEvent[];
  total?: number;
}

const ERROR_MESSAGES: Record<string, string> = {
  ADMIN_PERMISSION_DENIED: '当前账号没有系统管理权限',
  AUTHENTICATION_REQUIRED: '登录状态已失效，请重新登录',
  SANDBOX_CONFIGURATION_INVALID: 'Sandbox 配置不合法，请检查后重试',
  SANDBOX_DEFAULT_NOT_FOUND: '尚未配置该范围的默认 Provider',
  SANDBOX_INTERNAL: 'Sandbox 服务暂时不可用，请稍后重试',
  SANDBOX_POLICY_DENIED: '当前操作被 Sandbox 安全策略拒绝',
  SANDBOX_PROVIDER_ALREADY_EXISTS: 'Provider 名称已存在',
  SANDBOX_PROVIDER_DISABLED: 'Provider 已禁用，无法执行该操作',
  SANDBOX_PROVIDER_IN_USE: 'Provider 正在使用或仍是默认项，暂不能操作',
  SANDBOX_PROVIDER_NOT_FOUND: 'Provider 不存在或已被删除',
  SANDBOX_PROVIDER_UNHEALTHY: 'Provider 健康检查未通过',
  SANDBOX_SCOPE_UNSUPPORTED: 'Provider 不支持该运行范围',
  SANDBOX_UNAVAILABLE: 'Sandbox 控制面当前不可用',
  SANDBOX_VERSION_CONFLICT: '配置已被其他管理员更新，请刷新后重试',
};

const SAFE_AUDIT_METADATA = new Set([
  'scope',
  'changed_fields',
  'previous_status',
  'new_status',
  'health_code',
  'version',
]);

const SAFE_FIELD_ERROR_KEYS = new Set([
  'request',
  'credential',
  'scopes',
  'type',
]);

const safeText = (value: string, maxLength = 255) =>
  String(value || '')
    .replace(/[\u0000-\u001f\u007f-\u009f]/g, ' ')
    .replace(/https?:\/\/\S+/gi, '[redacted]')
    .replace(
      /\b(?:token|secret|credential|authorization)\s*[:=]\s*\S+/gi,
      '[redacted]',
    )
    .trim()
    .slice(0, maxLength);

const sanitizeFieldErrors = (fieldErrors?: SandboxFieldErrors) =>
  Object.fromEntries(
    Object.entries(fieldErrors || {})
      .filter(
        ([key, value]) =>
          SAFE_FIELD_ERROR_KEYS.has(key) && typeof value === 'string',
      )
      .map(([key, value]) => [key, safeText(value, 160)]),
  );

export class SandboxAPIError extends Error {
  readonly status: number;
  readonly errorCode: string;
  readonly fieldErrors: SandboxFieldErrors;

  constructor(
    status: number,
    errorCode: string,
    fieldErrors: SandboxFieldErrors = {},
  ) {
    super(ERROR_MESSAGES[errorCode] || 'Sandbox 请求失败，请稍后重试');
    this.name = 'SandboxAPIError';
    this.status = status;
    this.errorCode = errorCode;
    this.fieldErrors = sanitizeFieldErrors(fieldErrors);
  }
}

export const isSandboxConflict = (error: object): boolean =>
  error instanceof SandboxAPIError &&
  error.status === 409 &&
  error.errorCode === 'SANDBOX_VERSION_CONFLICT';

export const isSandboxPermissionError = (error: object): boolean =>
  error instanceof SandboxAPIError &&
  (error.status === 401 || error.status === 403);

const policyValue = <T>(
  canonical: T | undefined,
  legacy: T | undefined,
  fallback: T,
) => canonical ?? legacy ?? fallback;

const safeStringList = (value?: string[]) =>
  Array.isArray(value)
    ? value.filter(item => typeof item === 'string').map(item => String(item))
    : [];

const sanitizePolicy = (
  policy: SandboxRuntimePolicyWire,
): SandboxRuntimePolicy => ({
  timeout_seconds: policyValue(
    policy.timeout_seconds,
    policy.TimeoutSeconds,
    0,
  ),
  memory_limit_mb: policyValue(policy.memory_limit_mb, policy.MemoryLimitMB, 0),
  cpu_limit: policyValue(policy.cpu_limit, policy.CPULimit, 0),
  max_output_bytes: policyValue(
    policy.max_output_bytes,
    policy.MaxOutputBytes,
    0,
  ),
  max_concurrency: policyValue(
    policy.max_concurrency,
    policy.MaxConcurrency,
    0,
  ),
  allow_network: policyValue(policy.allow_network, policy.AllowNetwork, false),
  network_allowlist: safeStringList(
    policyValue(policy.network_allowlist, policy.NetworkAllowlist, []),
  ),
  allowed_env_names: safeStringList(policy.allowed_env_names),
  virtual_read_prefixes: safeStringList(policy.virtual_read_prefixes),
  virtual_write_prefixes: safeStringList(policy.virtual_write_prefixes),
  allowed_executables: safeStringList(policy.allowed_executables),
  ffi_enabled: Boolean(policy.ffi_enabled),
  node_modules_mode:
    policy.node_modules_mode === 'approved_directory'
      ? 'approved_directory'
      : 'disabled',
  node_modules_directory_ref: String(policy.node_modules_directory_ref || ''),
});

const sanitizeProvider = (provider: SandboxProviderWire): SandboxProvider => ({
  id: Number(provider.id),
  name: String(provider.name || ''),
  type: provider.type,
  endpoint_hint: String(provider.endpoint_hint || ''),
  credential_configured: Boolean(provider.credential_configured),
  credential_fingerprint: String(provider.credential_fingerprint || ''),
  active: Boolean(provider.active),
  needs_rewrap: Boolean(provider.needs_rewrap),
  scopes: Array.isArray(provider.scopes) ? provider.scopes.slice() : [],
  policy: sanitizePolicy(provider.policy || {}),
  status: provider.status,
  health: {
    status: provider.health?.status || 'unknown',
    capabilities: Array.isArray(provider.health?.capabilities)
      ? provider.health.capabilities.slice()
      : [],
    reason_code: String(provider.health?.reason_code || '').slice(0, 64),
    message: safeText(provider.health?.message || '', 255),
    latency_bucket: String(provider.health?.latency_bucket || '').slice(0, 64),
    checked_at: String(provider.health?.checked_at || '').startsWith('0001-')
      ? ''
      : String(provider.health?.checked_at || ''),
  },
  version: Number(provider.version),
  created_at: String(provider.created_at || ''),
  updated_at: String(provider.updated_at || ''),
});

const sanitizeAuditMetadata = (metadata: Record<string, string>) =>
  Object.fromEntries(
    Object.entries(metadata || {})
      .filter(
        ([key, value]) =>
          SAFE_AUDIT_METADATA.has(key) && typeof value === 'string',
      )
      .map(([key, value]) => [key, safeText(value, 256)]),
  );

const sanitizeAuditEvent = (event: SandboxAuditEvent): SandboxAuditEvent => ({
  id: Number(event.id),
  provider_id: Number(event.provider_id),
  actor_user_id: Number(event.actor_user_id),
  action: safeText(event.action, 64),
  result: safeText(event.result, 64),
  request_id: safeText(event.request_id, 48),
  metadata: sanitizeAuditMetadata(event.metadata),
  created_at: String(event.created_at || ''),
});

const requestSandbox = async <T>(
  url: string,
  init: RequestInit,
): Promise<T> => {
  const response = await fetch(url, { credentials: 'include', ...init });
  const envelope = (await response.json().catch(() => ({
    code: response.status,
    error_code: 'SANDBOX_INTERNAL',
    msg: '',
  }))) as SandboxEnvelope<T>;
  if (!response.ok || envelope.data === undefined || envelope.data === null) {
    throw new SandboxAPIError(
      response.status,
      envelope.error_code || 'SANDBOX_INTERNAL',
      envelope.field_errors,
    );
  }
  return envelope.data;
};

const createSecureRequestID = () => {
  const cryptoSource = globalThis.crypto;
  if (typeof cryptoSource?.randomUUID === 'function') {
    return cryptoSource.randomUUID();
  }
  if (typeof cryptoSource?.getRandomValues !== 'function') {
    throw new Error('Secure random source is unavailable');
  }
  const bytes = cryptoSource.getRandomValues(new Uint8Array(16));
  bytes[6] = (bytes[6] & 0x0f) | 0x40;
  bytes[8] = (bytes[8] & 0x3f) | 0x80;
  const hex = Array.from(bytes, value => value.toString(16).padStart(2, '0'));
  return [
    hex.slice(0, 4).join(''),
    hex.slice(4, 6).join(''),
    hex.slice(6, 8).join(''),
    hex.slice(8, 10).join(''),
    hex.slice(10).join(''),
  ].join('-');
};

const jsonRequest = (method: string, body: object, signal?: AbortSignal) => ({
  body: JSON.stringify(body),
  headers: {
    'content-type': 'application/json',
    'X-Request-ID': createSecureRequestID(),
  },
  method,
  signal,
});

export const listSandboxProviders = async (
  filters: SandboxProviderFilters,
  signal?: AbortSignal,
): Promise<SandboxProviderList> => {
  const query = new URLSearchParams();
  if (filters.keyword) {
    query.set('keyword', filters.keyword);
  }
  if (filters.type) {
    query.set('type', filters.type);
  }
  if (filters.status) {
    query.set('status', filters.status);
  }
  if (filters.health) {
    query.set('health', filters.health);
  }
  if (filters.scope) {
    query.set('scope', filters.scope);
  }
  query.set('offset', String(filters.offset));
  query.set('limit', String(filters.limit));
  const result = await requestSandbox<SandboxProviderListWire>(
    `/api/admin/sandboxes?${query.toString()}`,
    { method: 'GET', signal },
  );
  return {
    items: (result.items || []).map(sanitizeProvider),
    total: Number(result.total || 0),
  };
};

export const listSandboxProviderDefaults = async (
  signal?: AbortSignal,
): Promise<SandboxProviderDefaults> => {
  const result = await requestSandbox<SandboxProviderDefaults>(
    '/api/admin/sandboxes/defaults',
    { method: 'GET', signal },
  );
  return {
    items: (result.items || []).map(item => ({
      scope: item.scope,
      configured: Boolean(item.configured),
      provider_id: item.provider_id === null ? null : Number(item.provider_id),
      provider_name: String(item.provider_name || ''),
      provider_type: item.provider_type || '',
      provider_status: item.provider_status || '',
      version: Number(item.version || 0),
      provider_version: Number(item.provider_version || 0),
      updated_at: item.updated_at ? String(item.updated_at) : null,
    })),
  };
};

export const getSandboxProviderSummary = async (
  signal?: AbortSignal,
): Promise<SandboxProviderSummary> => {
  const result = await requestSandbox<SandboxProviderSummary>(
    '/api/admin/sandboxes/summary',
    { method: 'GET', signal },
  );
  return {
    total: Number(result.total || 0),
    enabled: Number(result.enabled || 0),
    unhealthy: Number(result.unhealthy || 0),
  };
};

const sanitizeCapability = (
  value: SandboxCapabilityState,
): SandboxCapabilityState => ({
  available: Boolean(value?.available),
  reason_code: String(value?.reason_code || '').slice(0, 64),
  message: safeText(value?.message || '', 160),
});

export const getSandboxCapabilities = async (
  signal?: AbortSignal,
): Promise<SandboxCapabilities> => {
  const result = await requestSandbox<SandboxCapabilities>(
    '/api/admin/sandboxes/capabilities',
    { method: 'GET', signal },
  );
  return {
    control_plane: sanitizeCapability(result.control_plane),
    local_debug: sanitizeCapability(result.local_debug),
  };
};

export const getSandboxSchedulerSettings = async (
  signal?: AbortSignal,
): Promise<SandboxSchedulerSettingsSnapshot> =>
  requestSandbox<SandboxSchedulerSettingsSnapshot>(
    '/api/admin/sandboxes/scheduler-settings',
    { method: 'GET', signal },
  );

export const updateSandboxSchedulerSettings = async (
  expectedVersion: number,
  settings: SandboxSchedulerSettings,
  signal?: AbortSignal,
): Promise<SandboxSchedulerSettingsUpdate> =>
  requestSandbox<SandboxSchedulerSettingsUpdate>(
    '/api/admin/sandboxes/scheduler-settings',
    jsonRequest('PUT', { expected_version: expectedVersion, settings }, signal),
  );

export const getSandboxRuntimeStatus = async (
  signal?: AbortSignal,
): Promise<SandboxRuntimeStatus> => {
  const status = await requestSandbox<SandboxRuntimeStatus>(
    '/api/admin/sandboxes/runtime-status',
    { method: 'GET', signal },
  );
  return {
    available: Boolean(status.available),
    desired_config_version: Number(status.desired_config_version || 0),
    applied_config_version: Number(status.applied_config_version || 0),
    queue_depth: Number(status.queue_depth || 0),
    queue_by_scope: Object.fromEntries(
      Object.entries(status.queue_by_scope || {}).filter(
        ([scope, value]) =>
          ['agent', 'appdev', 'mcp_stdio', 'plugin'].includes(scope) &&
          Number.isInteger(Number(value)) &&
          Number(value) >= 0,
      ),
    ) as Partial<Record<SandboxScope, number>>,
    active_slots: Number(status.active_slots || 0),
    slot_capacity: Number(status.slot_capacity || 0),
    queue_high_watermark: Number(status.queue_high_watermark || 0),
    draining_count: Number(status.draining_count || 0),
    quarantined_count: Number(status.quarantined_count || 0),
    idle_containers: Number(status.idle_containers || 0),
    active_containers: Number(status.active_containers || 0),
    memory_reserve_state: ['available', 'below_watermark', 'unknown'].includes(
      String(status.memory_reserve_state),
    )
      ? (status.memory_reserve_state as SandboxRuntimeStatus['memory_reserve_state'])
      : undefined,
    reason_code: String(status.reason_code || '').slice(0, 64) || undefined,
  };
};

export const createSandboxProvider = async (
  payload: SandboxCreateProviderPayload,
  signal?: AbortSignal,
) =>
  sanitizeProvider(
    await requestSandbox<SandboxProviderWire>(
      '/api/admin/sandboxes',
      jsonRequest('POST', payload, signal),
    ),
  );

export const getSandboxProvider = async (
  providerID: number,
  signal?: AbortSignal,
) =>
  sanitizeProvider(
    await requestSandbox<SandboxProviderWire>(
      `/api/admin/sandboxes/${providerID}`,
      { method: 'GET', signal },
    ),
  );

export const updateSandboxProvider = async (
  providerID: number,
  payload: SandboxUpdateProviderPayload,
  signal?: AbortSignal,
) =>
  sanitizeProvider(
    await requestSandbox<SandboxProviderWire>(
      `/api/admin/sandboxes/${providerID}`,
      jsonRequest('PUT', payload, signal),
    ),
  );

export const deleteSandboxProvider = (
  providerID: number,
  expectedVersion: number,
  signal?: AbortSignal,
) =>
  requestSandbox<SandboxProviderMutation>(
    `/api/admin/sandboxes/${providerID}`,
    jsonRequest('DELETE', { expected_version: expectedVersion }, signal),
  );

export const setSandboxProviderEnabled = async (
  providerID: number,
  expectedVersion: number,
  enabled: boolean,
  signal?: AbortSignal,
) =>
  sanitizeProvider(
    await requestSandbox<SandboxProviderWire>(
      `/api/admin/sandboxes/${providerID}/${enabled ? 'enable' : 'disable'}`,
      jsonRequest('POST', { expected_version: expectedVersion }, signal),
    ),
  );

export const replaceSandboxProviderCredential = async (
  providerID: number,
  expectedVersion: number,
  credential: string,
  signal?: AbortSignal,
) =>
  sanitizeProvider(
    await requestSandbox<SandboxProviderWire>(
      `/api/admin/sandboxes/${providerID}/credentials`,
      jsonRequest(
        'POST',
        {
          expected_version: expectedVersion,
          mode: 'replace',
          credential,
        },
        signal,
      ),
    ),
  );

export const clearSandboxProviderCredential = async (
  providerID: number,
  expectedVersion: number,
  signal?: AbortSignal,
) =>
  sanitizeProvider(
    await requestSandbox<SandboxProviderWire>(
      `/api/admin/sandboxes/${providerID}/credentials`,
      jsonRequest(
        'POST',
        { expected_version: expectedVersion, mode: 'clear' },
        signal,
      ),
    ),
  );

export const setSandboxProviderDefault = (
  providerID: number,
  expectedVersion: number,
  scope: SandboxScope,
  defaultExpectedVersion: number,
  signal?: AbortSignal,
) =>
  requestSandbox<SandboxProviderDefault>(
    `/api/admin/sandboxes/${providerID}/defaults`,
    jsonRequest(
      'POST',
      {
        expected_version: expectedVersion,
        default_expected_version: defaultExpectedVersion,
        scope,
      },
      signal,
    ),
  );

export const healthCheckSandboxProvider = async (
  providerID: number,
  expectedVersion: number,
  signal?: AbortSignal,
) =>
  sanitizeProvider(
    await requestSandbox<SandboxProviderWire>(
      `/api/admin/sandboxes/${providerID}/health`,
      jsonRequest('POST', { expected_version: expectedVersion }, signal),
    ),
  );

export const listSandboxAuditEvents = async (
  providerID: number,
  filters: SandboxAuditFilters,
  signal?: AbortSignal,
): Promise<SandboxAuditEventList> => {
  const query = new URLSearchParams();
  if (filters.action) {
    query.set('action', filters.action);
  }
  if (filters.result) {
    query.set('result', filters.result);
  }
  query.set('offset', String(filters.offset));
  query.set('limit', String(filters.limit));
  const result = await requestSandbox<SandboxAuditEventListWire>(
    `/api/admin/sandboxes/${providerID}/audit-events?${query.toString()}`,
    { method: 'GET', signal },
  );
  return {
    items: (result.items || []).map(sanitizeAuditEvent),
    total: Number(result.total || 0),
  };
};
