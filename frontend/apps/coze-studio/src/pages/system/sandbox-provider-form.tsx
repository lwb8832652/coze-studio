/* Copyright 2025 coze-dev Authors */

/* eslint-disable @coze-arch/max-line-per-function, max-lines-per-function -- Cohesive validated form. */
/* eslint-disable max-lines -- The policy contract is intentionally explicit. */

import { useCallback, useEffect, useRef, useState } from 'react';

import {
  getLocalDebugAvailability,
  SANDBOX_SCOPE_OPTIONS,
  splitPolicyList,
  validateSandboxRuntimePolicy,
} from './sandbox-view-model';
import {
  SandboxAPIError,
  type SandboxCapabilities,
  type SandboxProvider,
  type SandboxProviderType,
  type SandboxRuntimePolicy,
  type SandboxScope,
  type SandboxSecretMutation,
} from './sandbox-service';
import { useSandboxDialogFocus } from './sandbox-dialog-focus';

import styles from './sandbox-management-section.module.less';

const DEFAULT_POLICY: SandboxRuntimePolicy = {
  timeout_seconds: 60,
  memory_limit_mb: 512,
  cpu_limit: 1,
  max_output_bytes: 65536,
  max_concurrency: 8,
  allow_network: false,
  network_allowlist: [],
  allowed_env_names: [],
  virtual_read_prefixes: [],
  virtual_write_prefixes: [],
  allowed_executables: [],
  ffi_enabled: false,
  node_modules_mode: 'disabled',
  node_modules_directory_ref: '',
};

type PolicyListKey =
  | 'network_allowlist'
  | 'allowed_env_names'
  | 'virtual_read_prefixes'
  | 'virtual_write_prefixes'
  | 'allowed_executables';

const EMPTY_LIST_TEXT: Record<PolicyListKey, string> = {
  network_allowlist: '',
  allowed_env_names: '',
  virtual_read_prefixes: '',
  virtual_write_prefixes: '',
  allowed_executables: '',
};

export interface SandboxProviderFormValue {
  name: string;
  type: SandboxProviderType;
  endpoint: SandboxSecretMutation;
  credential: SandboxSecretMutation;
  scopes: SandboxScope[];
  policy: SandboxRuntimePolicy;
}

interface SandboxProviderFormProps {
  capabilities?: SandboxCapabilities;
  open: boolean;
  mode: 'create' | 'edit';
  provider?: SandboxProvider | null;
  onCancel: () => void;
  onSubmit: (value: SandboxProviderFormValue) => void | Promise<void>;
}

const toNumber = (value: string) => Number(value || 0);

const FieldError = ({ message }: { message?: string }) =>
  message ? <small className={styles.fieldError}>{message}</small> : null;

export const SandboxProviderForm = ({
  capabilities,
  open,
  mode,
  provider,
  onCancel,
  onSubmit,
}: SandboxProviderFormProps) => {
  const localDebug = getLocalDebugAvailability(capabilities);
  const [name, setName] = useState('');
  const [type, setType] = useState<SandboxProviderType>('remote_http');
  const [endpoint, setEndpoint] = useState('');
  const [credential, setCredential] = useState('');
  const [replaceEndpoint, setReplaceEndpoint] = useState(false);
  const [replaceCredential, setReplaceCredential] = useState(false);
  const [scopes, setScopes] = useState<SandboxScope[]>(['agent']);
  const [policy, setPolicy] = useState<SandboxRuntimePolicy>(DEFAULT_POLICY);
  const [listText, setListText] = useState(EMPTY_LIST_TEXT);
  const [fieldErrors, setFieldErrors] = useState<Record<string, string>>({});
  const [validationMessage, setValidationMessage] = useState('');
  const [submitting, setSubmitting] = useState(false);
  const submittingRef = useRef(false);
  const mountedRef = useRef(true);
  const openRef = useRef(open);
  const submissionGenerationRef = useRef(0);
  const nameInputRef = useRef<HTMLInputElement>(null);
  openRef.current = open;

  const clearSensitiveState = useCallback(() => {
    if (!mountedRef.current) {
      return;
    }
    setEndpoint('');
    setCredential('');
    setReplaceEndpoint(false);
    setReplaceCredential(false);
  }, []);

  const cancelForm = useCallback(() => {
    clearSensitiveState();
    onCancel();
  }, [clearSensitiveState, onCancel]);

  const { dialogRef, onDialogKeyDown } = useSandboxDialogFocus({
    canClose: !submitting,
    initialFocusRef: nameInputRef,
    onClose: cancelForm,
    open,
  });

  useEffect(
    () => () => {
      mountedRef.current = false;
      submittingRef.current = false;
      submissionGenerationRef.current += 1;
    },
    [],
  );

  useEffect(() => {
    submissionGenerationRef.current += 1;
    submittingRef.current = false;
    if (!open) {
      clearSensitiveState();
      setSubmitting(false);
      return;
    }
    const nextPolicy = provider?.policy || DEFAULT_POLICY;
    setName(provider?.name || '');
    setType(provider?.type || 'remote_http');
    clearSensitiveState();
    setScopes(provider?.scopes?.slice() || ['agent']);
    setPolicy({ ...nextPolicy });
    setListText({
      network_allowlist: nextPolicy.network_allowlist.join('\n'),
      allowed_env_names: nextPolicy.allowed_env_names.join('\n'),
      virtual_read_prefixes: nextPolicy.virtual_read_prefixes.join('\n'),
      virtual_write_prefixes: nextPolicy.virtual_write_prefixes.join('\n'),
      allowed_executables: nextPolicy.allowed_executables.join('\n'),
    });
    setFieldErrors({});
    setValidationMessage('');
    setSubmitting(false);
    submittingRef.current = false;
  }, [clearSensitiveState, open, provider]);

  if (!open) {
    return null;
  }

  const toggleScope = (scope: SandboxScope) => {
    setScopes(current =>
      current.includes(scope)
        ? current.filter(item => item !== scope)
        : [...current, scope],
    );
  };

  const updatePolicy = <K extends keyof SandboxRuntimePolicy>(
    key: K,
    value: SandboxRuntimePolicy[K],
  ) => setPolicy(current => ({ ...current, [key]: value }));

  const updateList = (key: PolicyListKey, value: string) =>
    setListText(current => ({ ...current, [key]: value }));

  const buildPolicy = (): SandboxRuntimePolicy => ({
    ...policy,
    network_allowlist: policy.allow_network
      ? splitPolicyList(listText.network_allowlist)
      : [],
    allowed_env_names: splitPolicyList(listText.allowed_env_names),
    virtual_read_prefixes: splitPolicyList(listText.virtual_read_prefixes),
    virtual_write_prefixes: splitPolicyList(listText.virtual_write_prefixes),
    allowed_executables: splitPolicyList(listText.allowed_executables),
    node_modules_directory_ref:
      policy.node_modules_mode === 'approved_directory'
        ? policy.node_modules_directory_ref.trim()
        : '',
  });

  const submit = async () => {
    if (submittingRef.current || !mountedRef.current || !openRef.current) {
      return;
    }
    const nextErrors: Record<string, string> = {};
    if (!name.trim()) {
      nextErrors.name = '请填写 Provider 名称';
    }
    if (name.trim().length > 128) {
      nextErrors.name = 'Provider 名称不能超过 128 个字符';
    }
    if (scopes.length === 0) {
      nextErrors.scopes = '请至少选择一个适用范围';
    }
    if (mode === 'create' && type === 'local_debug' && !localDebug.enabled) {
      nextErrors.type = localDebug.reason;
    }
    if (mode === 'create' && type === 'remote_http') {
      if (!/^https:\/\/[^\s]+$/i.test(endpoint.trim())) {
        nextErrors.endpoint = '远程 Provider Endpoint 必须使用 HTTPS';
      }
      if (!credential.trim()) {
        nextErrors.credential = '远程 Provider 必须填写凭据';
      }
    }
    if (replaceEndpoint && !/^https:\/\/[^\s]+$/i.test(endpoint.trim())) {
      nextErrors.endpoint = '新的 Provider Endpoint 必须使用 HTTPS';
    }
    if (replaceCredential && !credential.trim()) {
      nextErrors.credential = '请填写新的 Provider 凭据';
    }
    const finalPolicy = buildPolicy();
    for (const [key, message] of Object.entries(
      validateSandboxRuntimePolicy(finalPolicy),
    )) {
      if (message) {
        nextErrors[`policy.${key}`] = message;
      }
    }
    setFieldErrors(nextErrors);
    if (Object.keys(nextErrors).length) {
      setValidationMessage('请修正标记的配置后重试');
      return;
    }
    if (
      mode === 'edit' &&
      (replaceEndpoint || replaceCredential) &&
      !window.confirm(
        '确认替换 Provider 连接信息？旧值不会再次显示，提交后立即生效。',
      )
    ) {
      return;
    }

    setValidationMessage('');
    const submissionGeneration = submissionGenerationRef.current + 1;
    submissionGenerationRef.current = submissionGeneration;
    const isCurrentSubmission = () =>
      mountedRef.current &&
      openRef.current &&
      submissionGenerationRef.current === submissionGeneration;
    submittingRef.current = true;
    setSubmitting(true);
    try {
      await onSubmit({
        name: name.trim(),
        type,
        endpoint: {
          mode: mode === 'create' || replaceEndpoint ? 'replace' : 'keep',
          value: endpoint.trim(),
        },
        credential: {
          mode: mode === 'create' || replaceCredential ? 'replace' : 'keep',
          value: credential,
        },
        scopes,
        policy: finalPolicy,
      });
      if (!isCurrentSubmission()) {
        return;
      }
      clearSensitiveState();
    } catch (error) {
      if (!isCurrentSubmission()) {
        return;
      }
      if (error instanceof SandboxAPIError) {
        const {
          credential: credentialError,
          scopes: scopesError,
          type: typeError,
        } = error.fieldErrors;
        setFieldErrors(
          Object.fromEntries(
            Object.entries({
              credential: credentialError,
              scopes: scopesError,
              type: typeError,
            }).filter((entry): entry is [string, string] => Boolean(entry[1])),
          ),
        );
      }
      setValidationMessage(
        error instanceof SandboxAPIError && error.fieldErrors.request
          ? error.fieldErrors.request
          : error instanceof Error
            ? error.message
            : '保存失败，请重试',
      );
    } finally {
      if (isCurrentSubmission()) {
        submittingRef.current = false;
        setSubmitting(false);
      }
    }
  };

  return (
    <div className={styles.overlay} role="presentation">
      <section
        aria-labelledby="sandbox-provider-form-title"
        aria-modal="true"
        className={styles.drawer}
        ref={dialogRef}
        role="dialog"
        tabIndex={-1}
        onKeyDown={onDialogKeyDown}
      >
        <header className={styles.drawerHeader}>
          <div>
            <p className={styles.eyebrow}>Sandbox Provider</p>
            <h2 id="sandbox-provider-form-title">
              {mode === 'create' ? '创建运行 Provider' : '编辑运行 Provider'}
            </h2>
          </div>
          <button
            aria-label="关闭 Provider 表单"
            disabled={submitting}
            type="button"
            onClick={cancelForm}
          >
            关闭
          </button>
        </header>

        <div className={styles.formBody}>
          <label>
            <span>名称</span>
            <input
              ref={nameInputRef}
              aria-label="Provider 名称"
              disabled={submitting}
              value={name}
              onChange={event => setName(event.target.value)}
            />
            <FieldError message={fieldErrors.name} />
          </label>
          <label>
            <span>类型</span>
            <select
              aria-label="Provider 类型"
              disabled={submitting || mode === 'edit'}
              value={type}
              onChange={event =>
                setType(event.target.value as SandboxProviderType)
              }
            >
              <option value="remote_http">远程 HTTP</option>
              <option disabled={!localDebug.enabled} value="local_debug">
                本地调试
              </option>
            </select>
            <FieldError message={fieldErrors.type} />
          </label>
          <p
            className={
              localDebug.enabled ? styles.successNotice : styles.notice
            }
            role="status"
          >
            {localDebug.reason}。local-debug 可用性只以服务端 capability 为准。
          </p>

          {type === 'remote_http' ? (
            <fieldset className={styles.fieldset}>
              <legend>连接信息</legend>
              {mode === 'edit' ? (
                <label className={styles.checkline}>
                  <input
                    aria-label="替换 Provider Endpoint"
                    checked={replaceEndpoint}
                    disabled={submitting}
                    type="checkbox"
                    onChange={event => setReplaceEndpoint(event.target.checked)}
                  />
                  显式替换 Endpoint（当前仅显示{' '}
                  {provider?.endpoint_hint || '安全摘要'}）
                </label>
              ) : null}
              {mode === 'create' || replaceEndpoint ? (
                <label>
                  <span>Endpoint</span>
                  <input
                    aria-label="Provider Endpoint"
                    autoComplete="off"
                    disabled={submitting}
                    placeholder="https://runner.example.com"
                    value={endpoint}
                    onChange={event => setEndpoint(event.target.value)}
                  />
                  <small>仅接受 HTTPS；保存后只显示安全摘要。</small>
                  <FieldError message={fieldErrors.endpoint} />
                </label>
              ) : null}
              {mode === 'edit' ? (
                <div className={styles.secretSummary}>
                  <span>凭据指纹</span>
                  <strong>
                    {provider?.credential_fingerprint || '未配置'}
                  </strong>
                  <label className={styles.checkline}>
                    <input
                      aria-label="替换 Provider 凭据"
                      checked={replaceCredential}
                      disabled={submitting}
                      type="checkbox"
                      onChange={event =>
                        setReplaceCredential(event.target.checked)
                      }
                    />
                    显式替换凭据
                  </label>
                </div>
              ) : null}
              {mode === 'create' || replaceCredential ? (
                <label>
                  <span>{mode === 'create' ? '凭据' : '新凭据'}</span>
                  <input
                    aria-label={
                      mode === 'create' ? 'Provider 凭据' : '新的 Provider 凭据'
                    }
                    autoComplete="new-password"
                    disabled={submitting}
                    type="password"
                    value={credential}
                    onChange={event => setCredential(event.target.value)}
                  />
                  <FieldError message={fieldErrors.credential} />
                </label>
              ) : null}
            </fieldset>
          ) : null}

          <fieldset className={styles.fieldset}>
            <legend>适用范围</legend>
            <div className={styles.scopeGrid}>
              {SANDBOX_SCOPE_OPTIONS.map(option => (
                <label className={styles.scopeOption} key={option.value}>
                  <input
                    checked={scopes.includes(option.value)}
                    disabled={submitting}
                    type="checkbox"
                    onChange={() => toggleScope(option.value)}
                  />
                  <span>
                    <strong>{option.label}</strong>
                    <small>{option.description}</small>
                  </span>
                </label>
              ))}
            </div>
            <FieldError message={fieldErrors.scopes} />
          </fieldset>

          <fieldset className={styles.fieldset}>
            <legend>资源与容量</legend>
            <p className={styles.fieldHint}>
              边界与服务端领域合同一致，超限会在提交前阻止。
            </p>
            <div className={styles.policyGrid}>
              <label>
                <span>超时（秒）</span>
                <input
                  aria-label="运行超时秒数"
                  disabled={submitting}
                  min={1}
                  max={3600}
                  type="number"
                  value={policy.timeout_seconds}
                  onChange={event =>
                    updatePolicy(
                      'timeout_seconds',
                      toNumber(event.target.value),
                    )
                  }
                />
                <FieldError message={fieldErrors['policy.timeout_seconds']} />
              </label>
              <label>
                <span>内存（MB）</span>
                <input
                  aria-label="内存限制 MB"
                  disabled={submitting}
                  min={64}
                  max={32768}
                  type="number"
                  value={policy.memory_limit_mb}
                  onChange={event =>
                    updatePolicy(
                      'memory_limit_mb',
                      toNumber(event.target.value),
                    )
                  }
                />
                <FieldError message={fieldErrors['policy.memory_limit_mb']} />
              </label>
              <label>
                <span>CPU</span>
                <input
                  aria-label="CPU 限制"
                  disabled={submitting}
                  min={0.1}
                  max={64}
                  step={0.1}
                  type="number"
                  value={policy.cpu_limit}
                  onChange={event =>
                    updatePolicy('cpu_limit', toNumber(event.target.value))
                  }
                />
                <FieldError message={fieldErrors['policy.cpu_limit']} />
              </label>
              <label>
                <span>输出上限（bytes）</span>
                <input
                  aria-label="输出字节上限"
                  disabled={submitting}
                  min={1024}
                  max={16777216}
                  type="number"
                  value={policy.max_output_bytes}
                  onChange={event =>
                    updatePolicy(
                      'max_output_bytes',
                      toNumber(event.target.value),
                    )
                  }
                />
                <FieldError message={fieldErrors['policy.max_output_bytes']} />
              </label>
              <label>
                <span>最大并发</span>
                <input
                  aria-label="最大并发数"
                  disabled={submitting}
                  min={1}
                  max={1024}
                  type="number"
                  value={policy.max_concurrency}
                  onChange={event =>
                    updatePolicy(
                      'max_concurrency',
                      toNumber(event.target.value),
                    )
                  }
                />
                <FieldError message={fieldErrors['policy.max_concurrency']} />
              </label>
            </div>
          </fieldset>

          <fieldset className={styles.fieldset}>
            <legend>环境、文件与进程</legend>
            <label>
              <span>允许的环境变量名</span>
              <textarea
                aria-label="允许的环境变量名"
                disabled={submitting}
                rows={3}
                value={listText.allowed_env_names}
                onChange={event =>
                  updateList('allowed_env_names', event.target.value)
                }
              />
              <small>只填名称，例如 PATH；禁止填写 TOKEN=value。</small>
              <FieldError message={fieldErrors['policy.allowed_env_names']} />
            </label>
            <label>
              <span>虚拟只读前缀</span>
              <textarea
                aria-label="虚拟只读前缀"
                disabled={submitting}
                rows={3}
                value={listText.virtual_read_prefixes}
                onChange={event =>
                  updateList('virtual_read_prefixes', event.target.value)
                }
              />
              <small>
                仅 workspace、inputs、outputs、artifacts
                虚拟根，不接受宿主路径。
              </small>
              <FieldError
                message={fieldErrors['policy.virtual_read_prefixes']}
              />
            </label>
            <label>
              <span>虚拟可写前缀</span>
              <textarea
                aria-label="虚拟可写前缀"
                disabled={submitting}
                rows={3}
                value={listText.virtual_write_prefixes}
                onChange={event =>
                  updateList('virtual_write_prefixes', event.target.value)
                }
              />
              <FieldError
                message={fieldErrors['policy.virtual_write_prefixes']}
              />
            </label>
            <label>
              <span>允许的可执行文件</span>
              <textarea
                aria-label="允许的可执行文件"
                disabled={submitting}
                rows={3}
                value={listText.allowed_executables}
                onChange={event =>
                  updateList('allowed_executables', event.target.value)
                }
              />
              <small>只填可执行文件名，例如 node；禁止参数和命令字符串。</small>
              <FieldError message={fieldErrors['policy.allowed_executables']} />
            </label>
            <label className={styles.checkline}>
              <input
                aria-label="允许 FFI"
                checked={policy.ffi_enabled}
                disabled={submitting}
                type="checkbox"
                onChange={event =>
                  updatePolicy('ffi_enabled', event.target.checked)
                }
              />
              允许 FFI（高风险，仅在 Runner 策略同时允许时生效）
            </label>
          </fieldset>

          <fieldset className={styles.fieldset}>
            <legend>网络与 Node modules</legend>
            <label className={styles.checkline}>
              <input
                aria-label="允许 Provider 网络访问"
                checked={policy.allow_network}
                disabled={submitting}
                type="checkbox"
                onChange={event =>
                  updatePolicy('allow_network', event.target.checked)
                }
              />
              允许网络访问
            </label>
            {policy.allow_network ? (
              <label>
                <span>网络 allowlist</span>
                <textarea
                  aria-label="网络访问白名单"
                  disabled={submitting}
                  rows={4}
                  value={listText.network_allowlist}
                  onChange={event =>
                    updateList('network_allowlist', event.target.value)
                  }
                />
                <small>每行一个主机或 *.example.com。</small>
                <FieldError message={fieldErrors['policy.network_allowlist']} />
              </label>
            ) : null}
            <label>
              <span>Node modules 模式</span>
              <select
                aria-label="Node modules 模式"
                disabled={submitting}
                value={policy.node_modules_mode}
                onChange={event =>
                  updatePolicy(
                    'node_modules_mode',
                    event.target
                      .value as SandboxRuntimePolicy['node_modules_mode'],
                  )
                }
              >
                <option value="disabled">禁用</option>
                <option value="approved_directory">批准目录引用</option>
              </select>
            </label>
            {policy.node_modules_mode === 'approved_directory' ? (
              <label>
                <span>批准目录引用</span>
                <input
                  aria-label="Node modules 目录引用"
                  disabled={submitting}
                  value={policy.node_modules_directory_ref}
                  onChange={event =>
                    updatePolicy(
                      'node_modules_directory_ref',
                      event.target.value,
                    )
                  }
                />
                <small>填写受控目录标识，不是宿主绝对路径。</small>
                <FieldError
                  message={fieldErrors['policy.node_modules_directory_ref']}
                />
              </label>
            ) : null}
          </fieldset>

          {validationMessage ? (
            <p className={styles.formError} role="alert">
              {validationMessage}
            </p>
          ) : null}
        </div>

        <footer className={styles.drawerFooter}>
          <button
            aria-label="取消 Provider 表单"
            disabled={submitting}
            type="button"
            onClick={cancelForm}
          >
            取消
          </button>
          <button
            aria-label={mode === 'create' ? '创建 Provider' : '保存 Provider'}
            className={styles.primaryButton}
            disabled={submitting}
            type="button"
            onClick={() => void submit()}
          >
            {submitting
              ? '提交中...'
              : mode === 'create'
                ? '创建 Provider'
                : '保存变更'}
          </button>
        </footer>
      </section>
    </div>
  );
};
