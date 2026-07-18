/* Copyright 2025 coze-dev Authors */

import type {
  SandboxCapabilities,
  SandboxHealthProjection,
  SandboxProvider,
  SandboxProviderFilters,
  SandboxRuntimePolicy,
  SandboxScope,
} from './sandbox-service';

export const SANDBOX_SCOPE_OPTIONS: Array<{
  value: SandboxScope;
  label: string;
  description: string;
}> = [
  { value: 'agent', label: 'Agent', description: 'Agent 与工作流代码执行' },
  {
    value: 'mcp_stdio',
    label: 'MCP stdio',
    description: '本地协议型 MCP 进程',
  },
  {
    value: 'appdev',
    label: '网页应用开发',
    description: 'AppDev 构建与预览运行时',
  },
];

export const HEALTH_STALE_AFTER_MS = 5 * 60 * 1000;

export const getScopeLabel = (scope: SandboxScope) =>
  SANDBOX_SCOPE_OPTIONS.find(item => item.value === scope)?.label || scope;

export const getHealthLabel = (status: SandboxProvider['health']['status']) =>
  ({
    checking: '检查中',
    degraded: '性能下降',
    healthy: '健康',
    unhealthy: '异常',
    unknown: '未检查',
  })[status];

export const getProviderTypeLabel = (type: SandboxProvider['type']) =>
  type === 'remote_http' ? '远程 HTTP' : '本地调试';

export const getLocalDebugAvailability = (
  capabilities?: SandboxCapabilities,
) => ({
  enabled: Boolean(capabilities?.local_debug.available),
  reason:
    capabilities?.local_debug.message ||
    '服务端未声明本地调试能力，无法创建 local-debug Provider',
});

export const getHealthFreshness = (
  health: SandboxHealthProjection,
  now = new Date(),
) => {
  if (health.status === 'unknown' || !health.checked_at) {
    return { stale: true, label: '尚未完成健康检查' };
  }
  const checkedAt = new Date(health.checked_at).getTime();
  const age = now.getTime() - checkedAt;
  if (!Number.isFinite(checkedAt) || age < 0 || age > HEALTH_STALE_AFTER_MS) {
    return { stale: true, label: '健康结果已过期' };
  }
  return { stale: false, label: '健康结果有效' };
};

export const splitPolicyList = (value: string) =>
  value
    .split(/[\n,]/)
    .map(item => item.trim())
    .filter(
      (item, index, values) => Boolean(item) && values.indexOf(item) === index,
    );

const validVirtualPrefix = (value: string) => {
  if (
    !value ||
    value.length > 256 ||
    value.startsWith('/') ||
    /[\\:]/.test(value) ||
    !/^[A-Za-z0-9/_.-]+$/.test(value)
  ) {
    return false;
  }
  const segments = value.split('/');
  if (segments.some(segment => segment === '.' || segment === '..')) {
    return false;
  }
  return ['workspace', 'inputs', 'outputs', 'artifacts'].includes(segments[0]);
};

const validateList = (
  values: string[],
  validator: (value: string) => boolean,
  message: string,
) => (values.length <= 64 && values.every(validator) ? '' : message);

export type SandboxPolicyErrors = Partial<
  Record<keyof SandboxRuntimePolicy, string>
>;

export const validateSandboxRuntimePolicy = (
  policy: SandboxRuntimePolicy,
): SandboxPolicyErrors => {
  const errors: SandboxPolicyErrors = {};
  if (
    !Number.isInteger(policy.timeout_seconds) ||
    policy.timeout_seconds < 1 ||
    policy.timeout_seconds > 3600
  ) {
    errors.timeout_seconds = '超时范围为 1 到 3600 秒';
  }
  if (
    !Number.isInteger(policy.memory_limit_mb) ||
    policy.memory_limit_mb < 64 ||
    policy.memory_limit_mb > 32768
  ) {
    errors.memory_limit_mb = '内存范围为 64 到 32768 MB';
  }
  if (
    !Number.isFinite(policy.cpu_limit) ||
    policy.cpu_limit < 0.1 ||
    policy.cpu_limit > 64
  ) {
    errors.cpu_limit = 'CPU 范围为 0.1 到 64';
  }
  if (
    !Number.isInteger(policy.max_output_bytes) ||
    policy.max_output_bytes < 1024 ||
    policy.max_output_bytes > 16 * 1024 * 1024
  ) {
    errors.max_output_bytes = '输出范围为 1024 到 16777216 bytes';
  }
  if (
    !Number.isInteger(policy.max_concurrency) ||
    policy.max_concurrency < 1 ||
    policy.max_concurrency > 1024
  ) {
    errors.max_concurrency = '并发范围为 1 到 1024';
  }
  if (policy.allow_network) {
    const hostPattern =
      /^(?:\*\.)?(?:[a-z0-9](?:[a-z0-9-]{0,61}[a-z0-9])?)(?:\.(?:[a-z0-9](?:[a-z0-9-]{0,61}[a-z0-9])?))*$/;
    if (
      policy.network_allowlist.length === 0 ||
      policy.network_allowlist.length > 128 ||
      !policy.network_allowlist.every(item =>
        hostPattern.test(item.toLowerCase()),
      )
    ) {
      errors.network_allowlist = '网络白名单需填写合法主机名或 *.example.com';
    }
  } else if (policy.network_allowlist.length > 0) {
    errors.network_allowlist = '未启用网络访问时不能提交网络白名单';
  }
  const envError = validateList(
    policy.allowed_env_names,
    value => /^[A-Z_][A-Z0-9_]{0,127}$/.test(value),
    '环境变量名只能包含大写字母、数字和下划线，不能填写变量值',
  );
  if (envError) {
    errors.allowed_env_names = envError;
  }
  const readError = validateList(
    policy.virtual_read_prefixes,
    validVirtualPrefix,
    '虚拟路径必须从 workspace、inputs、outputs 或 artifacts 开始，且不能包含宿主路径或父目录',
  );
  if (readError) {
    errors.virtual_read_prefixes = readError;
  }
  const writeError = validateList(
    policy.virtual_write_prefixes,
    validVirtualPrefix,
    '虚拟路径必须从 workspace、inputs、outputs 或 artifacts 开始，且不能包含宿主路径或父目录',
  );
  if (writeError) {
    errors.virtual_write_prefixes = writeError;
  }
  const executableError = validateList(
    policy.allowed_executables,
    value => /^[A-Za-z0-9][A-Za-z0-9_.+-]{0,127}$/.test(value),
    '可执行文件只允许填写名称，不能包含参数或命令字符串',
  );
  if (executableError) {
    errors.allowed_executables = executableError;
  }
  if (
    policy.node_modules_mode === 'disabled' &&
    policy.node_modules_directory_ref
  ) {
    errors.node_modules_directory_ref = '禁用 Node modules 时不能填写目录引用';
  }
  if (
    policy.node_modules_mode === 'approved_directory' &&
    !/^[A-Za-z0-9][A-Za-z0-9_.-]{0,127}$/.test(
      policy.node_modules_directory_ref,
    )
  ) {
    errors.node_modules_directory_ref =
      '目录引用只能是受控标识，不能填写宿主绝对路径';
  }
  return errors;
};

export interface SandboxPendingMutation {
  action: string;
  providerID: number;
}

export interface SandboxViewState {
  phase: 'loading' | 'ready' | 'empty' | 'error';
  items: SandboxProvider[];
  total: number;
  errorMessage: string;
  message: string;
  conflict: boolean;
  pending: SandboxPendingMutation | null;
  filters: SandboxProviderFilters;
}

export type SandboxViewAction =
  | { type: 'load_started' }
  | { type: 'load_succeeded'; items: SandboxProvider[]; total: number }
  | { type: 'load_failed'; message: string }
  | { type: 'mutation_started'; action: string; providerID: number }
  | { type: 'mutation_succeeded'; message: string }
  | { type: 'mutation_failed'; message: string; conflict?: boolean }
  | { type: 'mutation_finished' }
  | { type: 'message_cleared' };

export const createSandboxViewState = (): SandboxViewState => ({
  phase: 'loading',
  items: [],
  total: 0,
  errorMessage: '',
  message: '',
  conflict: false,
  pending: null,
  filters: { offset: 0, limit: 20 },
});

export const sandboxViewReducer = (
  state: SandboxViewState,
  action: SandboxViewAction,
): SandboxViewState => {
  switch (action.type) {
    case 'load_started':
      return { ...state, phase: 'loading', errorMessage: '' };
    case 'load_succeeded':
      return {
        ...state,
        phase: action.items.length ? 'ready' : 'empty',
        items: action.items,
        total: action.total,
        errorMessage: '',
      };
    case 'load_failed':
      return {
        ...state,
        phase: 'error',
        errorMessage: action.message,
        items: [],
        total: 0,
      };
    case 'mutation_started':
      return {
        ...state,
        pending: { action: action.action, providerID: action.providerID },
        message: '',
        conflict: false,
      };
    case 'mutation_succeeded':
      return {
        ...state,
        pending: null,
        message: action.message,
        conflict: false,
      };
    case 'mutation_failed':
      return {
        ...state,
        pending: null,
        message: action.message,
        conflict: Boolean(action.conflict),
      };
    case 'mutation_finished':
      return { ...state, pending: null };
    case 'message_cleared':
      return { ...state, message: '', conflict: false };
    default:
      return state;
  }
};
