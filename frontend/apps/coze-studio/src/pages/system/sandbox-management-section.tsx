/* Copyright 2025 coze-dev Authors */

/* eslint-disable @coze-arch/max-line-per-function, max-lines-per-function, complexity, max-params -- Cohesive management page. */
/* eslint-disable max-lines -- The control-plane workflow is explicit. */

import { useCallback, useEffect, useReducer, useRef, useState } from 'react';

import {
  createSandboxViewState,
  getHealthFreshness,
  getHealthLabel,
  getProviderTypeLabel,
  getScopeLabel,
  SANDBOX_SCOPE_OPTIONS,
  sandboxViewReducer,
} from './sandbox-view-model';
import {
  createSandboxProvider,
  deleteSandboxProvider,
  getSandboxCapabilities,
  getSandboxProviderSummary,
  healthCheckSandboxProvider,
  isSandboxConflict,
  listSandboxProviderDefaults,
  listSandboxProviders,
  replaceSandboxProviderCredential,
  setSandboxProviderDefault,
  setSandboxProviderEnabled,
  updateSandboxProvider,
  type SandboxCapabilities,
  type SandboxHealthStatus,
  type SandboxProvider,
  type SandboxProviderDefaultProjection,
  type SandboxProviderFilters,
  type SandboxProviderStatus,
  type SandboxProviderSummary,
  type SandboxProviderType,
  type SandboxScope,
} from './sandbox-service';
import {
  SandboxProviderForm,
  type SandboxProviderFormValue,
} from './sandbox-provider-form';
import { SandboxProviderDetail } from './sandbox-provider-detail';
import { SandboxAuditDrawer } from './sandbox-audit-drawer';

import styles from './sandbox-management-section.module.less';

const PAGE_SIZE = 20;
const EMPTY_SUMMARY: SandboxProviderSummary = {
  total: 0,
  enabled: 0,
  unhealthy: 0,
};

const formatTime = (value: string | null) =>
  value ? new Date(value).toLocaleString('zh-CN', { hour12: false }) : '-';

const defaultsByScope = (items: SandboxProviderDefaultProjection[]) =>
  Object.fromEntries(items.map(item => [item.scope, item])) as Partial<
    Record<SandboxScope, SandboxProviderDefaultProjection>
  >;

const errorMessage = (error: object, fallback: string) =>
  error instanceof Error ? error.message : fallback;

export const SandboxManagementSection = () => {
  const [state, dispatch] = useReducer(
    sandboxViewReducer,
    undefined,
    createSandboxViewState,
  );
  const [keyword, setKeyword] = useState('');
  const [typeFilter, setTypeFilter] = useState<SandboxProviderType | ''>('');
  const [statusFilter, setStatusFilter] = useState<SandboxProviderStatus | ''>(
    '',
  );
  const [healthFilter, setHealthFilter] = useState<
    Exclude<SandboxHealthStatus, 'checking'> | ''
  >('');
  const [scopeFilter, setScopeFilter] = useState<SandboxScope | ''>('');
  const [query, setQuery] = useState<SandboxProviderFilters>({
    offset: 0,
    limit: PAGE_SIZE,
  });
  const [summary, setSummary] = useState<SandboxProviderSummary>(EMPTY_SUMMARY);
  const [capabilities, setCapabilities] = useState<SandboxCapabilities | null>(
    null,
  );
  const [defaults, setDefaults] = useState<
    Partial<Record<SandboxScope, SandboxProviderDefaultProjection>>
  >({});
  const [formMode, setFormMode] = useState<'create' | 'edit' | null>(null);
  const [editingProvider, setEditingProvider] =
    useState<SandboxProvider | null>(null);
  const [detailProvider, setDetailProvider] = useState<SandboxProvider | null>(
    null,
  );
  const [auditProvider, setAuditProvider] = useState<SandboxProvider | null>(
    null,
  );
  const [projectionStale, setProjectionStale] = useState(false);
  const pendingRef = useRef(false);
  const mountedRef = useRef(true);
  const mutationControllerRef = useRef<AbortController | null>(null);
  const projectionRequestRef = useRef<{
    controller: AbortController | null;
    generation: number;
  }>({ controller: null, generation: 0 });

  const dispatchIfMounted = useCallback(
    (action: Parameters<typeof dispatch>[0]) => {
      if (mountedRef.current) {
        dispatch(action);
      }
    },
    [],
  );

  const isMutationActive = useCallback(
    (controller: AbortController) =>
      mountedRef.current && !controller.signal.aborted,
    [],
  );

  const loadProjection = useCallback(
    async (
      filters: SandboxProviderFilters,
      showLoading = true,
      includeCapabilities = true,
    ) => {
      projectionRequestRef.current.controller?.abort();
      const controller = new AbortController();
      const generation = projectionRequestRef.current.generation + 1;
      projectionRequestRef.current = { controller, generation };
      const isCurrent = () =>
        mountedRef.current &&
        !controller.signal.aborted &&
        projectionRequestRef.current.generation === generation;
      if (showLoading) {
        dispatchIfMounted({ type: 'load_started' });
      }
      try {
        if (includeCapabilities) {
          const capabilityResult = await getSandboxCapabilities(
            controller.signal,
          );
          if (!isCurrent()) {
            return false;
          }
          setCapabilities(capabilityResult);
        }

        const [providers, defaultResult, summaryResult] = await Promise.all([
          listSandboxProviders(filters, controller.signal),
          listSandboxProviderDefaults(controller.signal),
          getSandboxProviderSummary(controller.signal),
        ]);
        if (!isCurrent()) {
          return false;
        }
        setDefaults(defaultsByScope(defaultResult.items));
        setSummary(summaryResult);
        setProjectionStale(false);
        dispatchIfMounted({
          type: 'load_succeeded',
          items: providers.items,
          total: providers.total,
        });
        return true;
      } catch (error) {
        const latest =
          mountedRef.current &&
          projectionRequestRef.current.generation === generation;
        if (!latest || controller.signal.aborted) {
          return false;
        }
        controller.abort();
        if (showLoading) {
          dispatchIfMounted({
            type: 'load_failed',
            message: errorMessage(
              error as object,
              '加载 Sandbox Provider 失败',
            ),
          });
        }
        throw error;
      } finally {
        if (projectionRequestRef.current.generation === generation) {
          projectionRequestRef.current.controller = null;
        }
      }
    },
    [dispatchIfMounted],
  );

  const refreshAffected = useCallback(
    async (mutationSignal?: AbortSignal) => {
      if (!mountedRef.current || mutationSignal?.aborted) {
        return false;
      }
      const refreshed = await loadProjection(query, false, false);
      if (!mountedRef.current || mutationSignal?.aborted) {
        return false;
      }
      return refreshed;
    },
    [loadProjection, query],
  );

  useEffect(() => {
    mountedRef.current = true;
    void loadProjection({ offset: 0, limit: PAGE_SIZE }).catch(() => undefined);
    return () => {
      mountedRef.current = false;
      projectionRequestRef.current.controller?.abort();
      projectionRequestRef.current = {
        controller: null,
        generation: projectionRequestRef.current.generation + 1,
      };
      mutationControllerRef.current?.abort();
    };
  }, [loadProjection]);

  const currentPage = Math.floor(query.offset / PAGE_SIZE) + 1;
  const mutationPending = state.pending !== null;
  const interactionBlocked = mutationPending || projectionStale;
  const configuredDefaultCount = SANDBOX_SCOPE_OPTIONS.filter(
    item => defaults[item.value]?.configured,
  ).length;

  const applyFilters = async () => {
    const next: SandboxProviderFilters = {
      keyword: keyword.trim() || undefined,
      type: typeFilter,
      status: statusFilter,
      health: healthFilter,
      scope: scopeFilter,
      offset: 0,
      limit: PAGE_SIZE,
    };
    setQuery(next);
    await loadProjection(next, true, false).catch(() => undefined);
  };

  const turnPage = async (offset: number) => {
    const next = { ...query, offset };
    setQuery(next);
    await loadProjection(next, true, false).catch(() => undefined);
  };

  const retryProjection = async () => {
    try {
      const refreshed = await loadProjection(query, true, true);
      if (refreshed) {
        dispatchIfMounted({
          type: 'mutation_succeeded',
          message: '最新 Sandbox 数据已刷新',
        });
      }
    } catch (error) {
      void error;
      // loadProjection owns the visible read error state.
    }
  };

  const refreshAfterSuccessfulWrite = async (
    successMessage: string,
    controller: AbortController,
  ) => {
    if (!isMutationActive(controller)) {
      return;
    }
    try {
      const refreshed = await refreshAffected(controller.signal);
      if (refreshed && isMutationActive(controller)) {
        dispatchIfMounted({
          type: 'mutation_succeeded',
          message: successMessage,
        });
      }
    } catch {
      if (!isMutationActive(controller)) {
        return;
      }
      setProjectionStale(true);
      dispatchIfMounted({
        type: 'mutation_failed',
        message: `${successMessage}。操作已成功，但刷新失败，可重试`,
      });
    }
  };

  const runMutation = async (
    provider: SandboxProvider,
    action: string,
    successMessage: string,
    operation: (signal: AbortSignal) => Promise<object>,
  ) => {
    if (pendingRef.current) {
      return;
    }
    pendingRef.current = true;
    dispatchIfMounted({
      type: 'mutation_started',
      providerID: provider.id,
      action,
    });
    const controller = new AbortController();
    mutationControllerRef.current = controller;
    try {
      await operation(controller.signal);
      if (!isMutationActive(controller)) {
        return;
      }
      await refreshAfterSuccessfulWrite(successMessage, controller);
    } catch (error) {
      if (!isMutationActive(controller)) {
        return;
      }
      const conflict = isSandboxConflict(error as object);
      if (conflict) {
        try {
          await refreshAffected(controller.signal);
          if (!isMutationActive(controller)) {
            return;
          }
        } catch {
          if (!isMutationActive(controller)) {
            return;
          }
          setProjectionStale(true);
          dispatchIfMounted({
            type: 'mutation_failed',
            message: `${errorMessage(error as object, '配置已冲突')}，且刷新失败，可重试`,
            conflict: true,
          });
          return;
        }
      }
      dispatchIfMounted({
        type: 'mutation_failed',
        message: errorMessage(error as object, '操作失败，请重试'),
        conflict,
      });
    } finally {
      pendingRef.current = false;
      if (mutationControllerRef.current === controller) {
        mutationControllerRef.current = null;
      }
      if (isMutationActive(controller)) {
        dispatchIfMounted({ type: 'mutation_finished' });
      }
    }
  };

  const closeForm = () => {
    if (!mountedRef.current) {
      return;
    }
    setFormMode(null);
    setEditingProvider(null);
  };

  const handleFormSubmit = async (value: SandboxProviderFormValue) => {
    if (pendingRef.current || !mountedRef.current || !formMode) {
      return;
    }
    const submittingMode = formMode;
    const submittingProvider = editingProvider;
    pendingRef.current = true;
    dispatchIfMounted({
      type: 'mutation_started',
      providerID: submittingProvider?.id || 0,
      action: submittingMode,
    });
    const controller = new AbortController();
    mutationControllerRef.current = controller;
    try {
      if (submittingMode === 'create') {
        const created = await createSandboxProvider(
          {
            name: value.name,
            type: value.type,
            endpoint: value.endpoint.value,
            credential: value.credential.value,
            scopes: value.scopes,
            policy: value.policy,
          },
          controller.signal,
        );
        if (!isMutationActive(controller)) {
          return;
        }
        closeForm();
        await refreshAfterSuccessfulWrite(
          `Provider ${created.name} 已创建`,
          controller,
        );
        return;
      }
      if (!submittingProvider) {
        return;
      }
      let updated: SandboxProvider;
      try {
        updated = await updateSandboxProvider(
          submittingProvider.id,
          {
            expected_version: submittingProvider.version,
            name: value.name,
            scopes: value.scopes,
            policy: value.policy,
            endpoint: value.endpoint,
            credential: { mode: 'keep', value: '' },
          },
          controller.signal,
        );
      } catch (error) {
        if (!isMutationActive(controller)) {
          return;
        }
        if (isSandboxConflict(error as object)) {
          closeForm();
          try {
            await refreshAffected(controller.signal);
            if (!isMutationActive(controller)) {
              return;
            }
            dispatchIfMounted({
              type: 'mutation_failed',
              message: errorMessage(
                error as object,
                '配置已被其他管理员更新，请刷新后重试',
              ),
              conflict: true,
            });
          } catch {
            if (!isMutationActive(controller)) {
              return;
            }
            setProjectionStale(true);
            dispatchIfMounted({
              type: 'mutation_failed',
              message: '配置已被其他管理员更新，且刷新失败，可重试',
              conflict: true,
            });
          }
          return;
        }
        dispatchIfMounted({
          type: 'mutation_failed',
          message: errorMessage(error as object, '保存失败'),
        });
        throw error;
      }
      if (!isMutationActive(controller)) {
        return;
      }
      if (value.credential.mode === 'replace') {
        try {
          await replaceSandboxProviderCredential(
            updated.id,
            updated.version,
            value.credential.value,
            controller.signal,
          );
        } catch (error) {
          if (!isMutationActive(controller)) {
            return;
          }
          closeForm();
          try {
            await refreshAffected(controller.signal);
            if (!isMutationActive(controller)) {
              return;
            }
            dispatchIfMounted({
              type: 'mutation_failed',
              message:
                '基础配置已保存，但凭据替换失败。已刷新最新数据，请重新进入编辑后重试凭据替换。',
              conflict: isSandboxConflict(error as object),
            });
          } catch {
            if (!isMutationActive(controller)) {
              return;
            }
            setProjectionStale(true);
            dispatchIfMounted({
              type: 'mutation_failed',
              message: '基础配置已保存，但凭据替换失败，且刷新失败，可重试',
              conflict: isSandboxConflict(error as object),
            });
          }
          return;
        }
      }
      if (!isMutationActive(controller)) {
        return;
      }
      closeForm();
      await refreshAfterSuccessfulWrite('Provider 配置已更新', controller);
    } catch (error) {
      if (!isMutationActive(controller)) {
        return;
      }
      if (submittingMode === 'create') {
        dispatchIfMounted({
          type: 'mutation_failed',
          message: errorMessage(error as object, '创建 Provider 失败'),
          conflict: isSandboxConflict(error as object),
        });
      }
      throw error;
    } finally {
      pendingRef.current = false;
      if (mutationControllerRef.current === controller) {
        mutationControllerRef.current = null;
      }
      if (isMutationActive(controller)) {
        dispatchIfMounted({ type: 'mutation_finished' });
      }
    }
  };

  const setDefault = async (provider: SandboxProvider, scope: SandboxScope) => {
    if (
      !window.confirm(
        `确认将 ${provider.name} 设置为 ${getScopeLabel(scope)} 默认 Provider？`,
      )
    ) {
      return;
    }
    await runMutation(
      provider,
      `default:${scope}`,
      `${getScopeLabel(scope)} 默认 Provider 已更新`,
      signal =>
        setSandboxProviderDefault(
          provider.id,
          provider.version,
          scope,
          defaults[scope]?.version || 0,
          signal,
        ),
    );
  };

  return (
    <section
      aria-busy={state.phase === 'loading' || mutationPending}
      className={styles.page}
    >
      <div className={styles.topbar}>
        <div>
          <p className={styles.eyebrow}>Sandbox control plane</p>
          <h2>运行 Provider</h2>
          <p>
            生产流量只使用服务端选择的健康 Provider；配置缺失时保持
            fail-closed。
          </p>
        </div>
        <button
          aria-label="创建 Sandbox Provider"
          className={styles.primaryButton}
          disabled={
            interactionBlocked ||
            capabilities?.control_plane.available === false
          }
          type="button"
          onClick={() => {
            setEditingProvider(null);
            setFormMode('create');
          }}
        >
          创建 Provider
        </button>
      </div>

      {capabilities ? (
        <div
          className={
            capabilities.control_plane.available
              ? styles.capabilityBanner
              : styles.warningBanner
          }
          role="status"
        >
          <div>
            <strong>
              {capabilities.control_plane.available
                ? '控制面可用'
                : '控制面不可用'}
            </strong>
            <span>{capabilities.control_plane.message}</span>
          </div>
          <small>{capabilities.control_plane.reason_code}</small>
        </div>
      ) : null}

      <div className={styles.summaryGrid} aria-label="Sandbox 全局汇总">
        <article>
          <span>Provider 总数</span>
          <strong>{summary.total}</strong>
          <small>服务端全局统计</small>
        </article>
        <article>
          <span>已启用</span>
          <strong>{summary.enabled}</strong>
          <small>可参与受控调度</small>
        </article>
        <article>
          <span>异常</span>
          <strong>{summary.unhealthy}</strong>
          <small>仅统计 unhealthy</small>
        </article>
        <article>
          <span>默认范围</span>
          <strong>{configuredDefaultCount}/3</strong>
          <small>Agent / MCP / AppDev</small>
        </article>
      </div>

      <section className={styles.defaultsPanel} aria-label="默认 Provider 范围">
        <header>
          <div>
            <h3>默认运行范围</h3>
            <p>默认项与 CAS 版本来自服务端投影，刷新后仍可安全修改。</p>
          </div>
        </header>
        <div className={styles.defaultsGrid}>
          {SANDBOX_SCOPE_OPTIONS.map(option => {
            const selected = defaults[option.value];
            return (
              <article key={option.value}>
                <span>{option.label}</span>
                <strong>
                  {selected?.configured ? selected.provider_name : '未配置'}
                </strong>
                <small>
                  {selected?.configured
                    ? `${selected.provider_status === 'enabled' ? '已启用' : '已禁用'} · 默认版本 v${selected.version}`
                    : '运行时将 fail-closed'}
                </small>
              </article>
            );
          })}
        </div>
      </section>

      <section className={styles.filters} aria-label="筛选 Sandbox Provider">
        <input
          aria-label="搜索 Sandbox Provider"
          disabled={interactionBlocked}
          placeholder="搜索名称"
          value={keyword}
          onChange={event => setKeyword(event.target.value)}
          onKeyDown={event => {
            if (event.key === 'Enter') {
              void applyFilters();
            }
          }}
        />
        <select
          aria-label="按 Provider 类型筛选"
          disabled={interactionBlocked}
          value={typeFilter}
          onChange={event =>
            setTypeFilter(event.target.value as SandboxProviderType | '')
          }
        >
          <option value="">全部类型</option>
          <option value="remote_http">远程 HTTP</option>
          <option value="local_debug">本地调试</option>
        </select>
        <select
          aria-label="按 Provider 状态筛选"
          disabled={interactionBlocked}
          value={statusFilter}
          onChange={event =>
            setStatusFilter(event.target.value as SandboxProviderStatus | '')
          }
        >
          <option value="">全部状态</option>
          <option value="enabled">已启用</option>
          <option value="disabled">已禁用</option>
        </select>
        <select
          aria-label="按健康状态筛选"
          disabled={interactionBlocked}
          value={healthFilter}
          onChange={event =>
            setHealthFilter(
              event.target.value as
                | Exclude<SandboxHealthStatus, 'checking'>
                | '',
            )
          }
        >
          <option value="">全部健康状态</option>
          <option value="unknown">未检查</option>
          <option value="healthy">健康</option>
          <option value="degraded">性能下降</option>
          <option value="unhealthy">异常</option>
        </select>
        <select
          aria-label="按 Provider 范围筛选"
          disabled={interactionBlocked}
          value={scopeFilter}
          onChange={event =>
            setScopeFilter(event.target.value as SandboxScope | '')
          }
        >
          <option value="">全部范围</option>
          {SANDBOX_SCOPE_OPTIONS.map(item => (
            <option key={item.value} value={item.value}>
              {item.label}
            </option>
          ))}
        </select>
        <button
          aria-label="应用 Sandbox 筛选"
          disabled={interactionBlocked}
          type="button"
          onClick={() => void applyFilters()}
        >
          筛选
        </button>
        <button
          aria-label="刷新 Sandbox Provider"
          disabled={interactionBlocked}
          type="button"
          onClick={() =>
            void loadProjection(query, true, true).catch(() => undefined)
          }
        >
          刷新
        </button>
      </section>

      {state.message ? (
        <div
          className={
            state.conflict || projectionStale
              ? styles.warningBanner
              : styles.successBanner
          }
          role="status"
        >
          <span>{state.message}</span>
          {state.conflict || projectionStale ? (
            <button
              aria-label="重试刷新 Sandbox 投影"
              type="button"
              onClick={() => void retryProjection()}
            >
              {projectionStale ? '重试刷新数据' : '刷新最新数据'}
            </button>
          ) : null}
        </div>
      ) : null}

      {state.phase === 'loading' ? (
        <div className={styles.loadingState} role="status">
          正在加载 Sandbox Provider...
        </div>
      ) : null}
      {state.phase === 'error' ? (
        <div className={styles.errorState} role="alert">
          <strong>加载 Sandbox Provider 失败</strong>
          <p>{state.errorMessage}</p>
          <button
            aria-label="重试加载 Sandbox Provider"
            type="button"
            onClick={() =>
              void loadProjection(query, true, true).catch(() => undefined)
            }
          >
            重新加载
          </button>
        </div>
      ) : null}
      {state.phase === 'empty' ? (
        <div className={styles.emptyState}>
          <strong>暂无 Sandbox Provider</strong>
          <p>当前筛选条件下没有 Provider，可调整筛选或创建远程 Provider。</p>
        </div>
      ) : null}

      {state.items.length ? (
        <div className={styles.tableWrap}>
          <table>
            <thead>
              <tr>
                <th>名称</th>
                <th>类型</th>
                <th>适用范围</th>
                <th>Endpoint</th>
                <th>默认范围</th>
                <th>健康</th>
                <th>容量</th>
                <th>更新时间</th>
                <th>操作</th>
              </tr>
            </thead>
            <tbody>
              {state.items.map(provider => {
                const isPending = state.pending?.providerID === provider.id;
                const freshness = getHealthFreshness(provider.health);
                const defaultScopes = provider.scopes.filter(
                  scope => defaults[scope]?.provider_id === provider.id,
                );
                return (
                  <tr key={provider.id}>
                    <td>
                      <button
                        className={styles.linkButton}
                        type="button"
                        onClick={() => setDetailProvider(provider)}
                      >
                        {provider.name}
                      </button>
                      <small>
                        v{provider.version} ·{' '}
                        {provider.status === 'enabled' ? '已启用' : '已禁用'}
                      </small>
                    </td>
                    <td>
                      {getProviderTypeLabel(provider.type)}
                      {provider.type === 'local_debug' ? (
                        <small>受服务端 debug 双门禁限制</small>
                      ) : null}
                    </td>
                    <td>
                      <div className={styles.tags}>
                        {provider.scopes.map(scope => (
                          <span key={scope}>{getScopeLabel(scope)}</span>
                        ))}
                      </div>
                    </td>
                    <td>
                      <span>{provider.endpoint_hint || '-'}</span>
                      <small>
                        {provider.credential_fingerprint || '未配置凭据'}
                      </small>
                    </td>
                    <td>
                      <div className={styles.tags}>
                        {defaultScopes.map(scope => (
                          <span key={scope}>{getScopeLabel(scope)}</span>
                        ))}
                        {defaultScopes.length === 0 ? (
                          <small>非默认</small>
                        ) : null}
                      </div>
                    </td>
                    <td>
                      <span
                        className={styles.health}
                        data-health={provider.health.status}
                      >
                        {isPending && state.pending?.action === 'health'
                          ? '检查中'
                          : getHealthLabel(provider.health.status)}
                      </span>
                      <small>{formatTime(provider.health.checked_at)}</small>
                      {provider.health.message ? (
                        <small>{provider.health.message}</small>
                      ) : null}
                      {freshness.stale ? (
                        <small className={styles.staleWarning}>
                          {freshness.label}
                        </small>
                      ) : null}
                    </td>
                    <td>
                      <span>{provider.policy.max_concurrency} 并发</span>
                      <small>
                        {provider.policy.memory_limit_mb} MB ·{' '}
                        {provider.policy.timeout_seconds}s
                      </small>
                    </td>
                    <td>{formatTime(provider.updated_at)}</td>
                    <td>
                      <div className={styles.actions}>
                        <button
                          aria-label={`编辑 ${provider.name}`}
                          disabled={interactionBlocked}
                          type="button"
                          onClick={() => {
                            setEditingProvider(provider);
                            setFormMode('edit');
                          }}
                        >
                          编辑
                        </button>
                        <button
                          aria-label={`检查 ${provider.name} 健康状态`}
                          disabled={interactionBlocked}
                          type="button"
                          onClick={() =>
                            void runMutation(
                              provider,
                              'health',
                              '健康检查已完成',
                              signal =>
                                healthCheckSandboxProvider(
                                  provider.id,
                                  provider.version,
                                  signal,
                                ),
                            )
                          }
                        >
                          健康检查
                        </button>
                        {provider.scopes.map(scope => (
                          <button
                            aria-label={`将 ${provider.name} 设为 ${getScopeLabel(scope)} 默认 Provider`}
                            disabled={
                              interactionBlocked ||
                              provider.status !== 'enabled' ||
                              provider.health.status !== 'healthy' ||
                              freshness.stale
                            }
                            key={scope}
                            type="button"
                            onClick={() => void setDefault(provider, scope)}
                          >
                            设为 {getScopeLabel(scope)} 默认
                          </button>
                        ))}
                        <button
                          aria-label={
                            provider.status === 'enabled'
                              ? `禁用并排空 ${provider.name}`
                              : `启用 ${provider.name}`
                          }
                          disabled={
                            interactionBlocked ||
                            (provider.status === 'disabled' &&
                              (provider.health.status !== 'healthy' ||
                                freshness.stale))
                          }
                          type="button"
                          onClick={() => {
                            const enabling = provider.status !== 'enabled';
                            const confirmation = enabling
                              ? `确认启用 ${provider.name}？启用后该 Provider 可承接新运行任务。`
                              : `确认禁用并排空 ${provider.name}？新任务将不再分配到该 Provider。`;
                            if (window.confirm(confirmation)) {
                              void runMutation(
                                provider,
                                'status',
                                enabling
                                  ? 'Provider 已启用'
                                  : 'Provider 已禁用并进入排空流程',
                                signal =>
                                  setSandboxProviderEnabled(
                                    provider.id,
                                    provider.version,
                                    enabling,
                                    signal,
                                  ),
                              );
                            }
                          }}
                        >
                          {provider.status === 'enabled'
                            ? '禁用 / 排空'
                            : '启用'}
                        </button>
                        <button
                          aria-label={`查看 ${provider.name} 审计`}
                          type="button"
                          onClick={() => setAuditProvider(provider)}
                        >
                          审计
                        </button>
                        <button
                          className={styles.dangerButton}
                          aria-label={`删除 ${provider.name}`}
                          disabled={interactionBlocked}
                          type="button"
                          onClick={() => {
                            if (
                              window.confirm(
                                `确认删除 ${provider.name}？默认项或活跃 workload 会由服务端阻止。`,
                              )
                            ) {
                              void runMutation(
                                provider,
                                'delete',
                                'Provider 已删除',
                                signal =>
                                  deleteSandboxProvider(
                                    provider.id,
                                    provider.version,
                                    signal,
                                  ),
                              );
                            }
                          }}
                        >
                          删除
                        </button>
                      </div>
                    </td>
                  </tr>
                );
              })}
            </tbody>
          </table>
        </div>
      ) : null}

      <footer className={styles.pagination}>
        <span>
          第 {currentPage} 页 · 共 {state.total} 个 Provider
        </span>
        <button
          aria-label="Sandbox Provider 上一页"
          disabled={query.offset === 0 || interactionBlocked}
          type="button"
          onClick={() => void turnPage(Math.max(0, query.offset - PAGE_SIZE))}
        >
          上一页
        </button>
        <button
          aria-label="Sandbox Provider 下一页"
          disabled={
            query.offset + PAGE_SIZE >= state.total || interactionBlocked
          }
          type="button"
          onClick={() => void turnPage(query.offset + PAGE_SIZE)}
        >
          下一页
        </button>
      </footer>

      <SandboxProviderForm
        capabilities={capabilities || undefined}
        mode={formMode || 'create'}
        open={formMode !== null}
        provider={editingProvider}
        onCancel={closeForm}
        onSubmit={handleFormSubmit}
      />
      <SandboxProviderDetail
        provider={detailProvider}
        onClose={() => setDetailProvider(null)}
      />
      <SandboxAuditDrawer
        provider={auditProvider}
        onClose={() => setAuditProvider(null)}
      />
    </section>
  );
};
