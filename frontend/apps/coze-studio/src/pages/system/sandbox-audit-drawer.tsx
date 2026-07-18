/* Copyright 2025 coze-dev Authors */

/* eslint-disable @coze-arch/max-line-per-function -- Drawer state and request fencing remain colocated. */

import { useCallback, useEffect, useRef, useState } from 'react';

import {
  listSandboxAuditEvents,
  type SandboxAuditEvent,
  type SandboxProvider,
} from './sandbox-service';
import { useSandboxDialogFocus } from './sandbox-dialog-focus';

import styles from './sandbox-management-section.module.less';

interface SandboxAuditDrawerProps {
  provider: SandboxProvider | null;
  onClose: () => void;
}

const PAGE_SIZE = 20;

const formatCorrelationID = (value: string) => {
  const normalized = value.trim();
  if (!normalized) {
    return '-';
  }
  if (normalized.length <= 12) {
    return normalized;
  }
  return `${normalized.slice(0, 8)}...${normalized.slice(-4)}`;
};

export const SandboxAuditDrawer = ({
  provider,
  onClose,
}: SandboxAuditDrawerProps) => {
  const [items, setItems] = useState<SandboxAuditEvent[]>([]);
  const [total, setTotal] = useState(0);
  const [action, setAction] = useState('');
  const [result, setResult] = useState('');
  const [page, setPage] = useState(1);
  const [loading, setLoading] = useState(false);
  const [error, setError] = useState('');
  const closeRef = useRef<HTMLButtonElement>(null);
  const mountedRef = useRef(true);
  const providerIDRef = useRef(provider?.id);
  const requestRef = useRef<{
    controller: AbortController | null;
    generation: number;
  }>({ controller: null, generation: 0 });
  providerIDRef.current = provider?.id;

  const { dialogRef, onDialogKeyDown } = useSandboxDialogFocus({
    initialFocusRef: closeRef,
    onClose,
    open: Boolean(provider),
  });

  const cancelActiveRequest = useCallback(() => {
    requestRef.current.controller?.abort();
    requestRef.current = {
      controller: null,
      generation: requestRef.current.generation + 1,
    };
  }, []);

  useEffect(() => {
    mountedRef.current = true;
    return () => {
      mountedRef.current = false;
      cancelActiveRequest();
    };
  }, [cancelActiveRequest]);

  useEffect(() => {
    if (!provider) {
      return;
    }
    setItems([]);
    setTotal(0);
    setAction('');
    setResult('');
    setPage(1);
    setError('');
  }, [provider?.id]);

  const load = useCallback(async () => {
    if (!provider) {
      return;
    }
    const providerID = provider.id;
    requestRef.current.controller?.abort();
    const controller = new AbortController();
    const generation = requestRef.current.generation + 1;
    requestRef.current = { controller, generation };
    const isCurrent = () =>
      mountedRef.current &&
      !controller.signal.aborted &&
      requestRef.current.generation === generation &&
      providerIDRef.current === providerID;
    setLoading(true);
    setError('');
    try {
      const response = await listSandboxAuditEvents(
        providerID,
        {
          action: action || undefined,
          result: result || undefined,
          offset: (page - 1) * PAGE_SIZE,
          limit: PAGE_SIZE,
        },
        controller.signal,
      );
      if (!isCurrent()) {
        return;
      }
      setItems(response.items);
      setTotal(response.total);
    } catch (requestError) {
      if (isCurrent()) {
        setError(
          requestError instanceof Error
            ? requestError.message
            : '加载审计记录失败',
        );
      }
    } finally {
      if (isCurrent()) {
        setLoading(false);
      }
    }
  }, [action, page, provider?.id, result]);

  useEffect(() => {
    void load();
    return cancelActiveRequest;
  }, [cancelActiveRequest, load]);

  if (!provider) {
    return null;
  }

  return (
    <div className={styles.overlay} role="presentation">
      <section
        aria-busy={loading}
        aria-labelledby="sandbox-audit-title"
        aria-modal="true"
        className={styles.wideDrawer}
        ref={dialogRef}
        role="dialog"
        tabIndex={-1}
        onKeyDown={onDialogKeyDown}
      >
        <header className={styles.drawerHeader}>
          <div>
            <p className={styles.eyebrow}>Sanitized audit</p>
            <h2 id="sandbox-audit-title">{provider.name} · 审计记录</h2>
          </div>
          <button
            ref={closeRef}
            aria-label="关闭 Sandbox 审计"
            type="button"
            onClick={onClose}
          >
            关闭
          </button>
        </header>
        <div className={styles.auditFilters}>
          <input
            aria-label="筛选审计动作"
            placeholder="动作，例如 health_check"
            value={action}
            onChange={event => {
              setAction(event.target.value);
              setPage(1);
            }}
          />
          <select
            aria-label="筛选审计结果"
            value={result}
            onChange={event => {
              setResult(event.target.value);
              setPage(1);
            }}
          >
            <option value="">全部结果</option>
            <option value="success">成功</option>
            <option value="failure">失败</option>
          </select>
          <button
            aria-label="刷新 Sandbox 审计"
            disabled={loading}
            type="button"
            onClick={() => void load()}
          >
            刷新
          </button>
        </div>
        {loading ? (
          <p className={styles.loadingInline} role="status">
            正在加载审计记录...
          </p>
        ) : null}
        {error ? (
          <div className={styles.errorState} role="alert">
            <p>{error}</p>
            <button
              aria-label="重试加载 Sandbox 审计"
              type="button"
              onClick={() => void load()}
            >
              重试
            </button>
          </div>
        ) : null}
        {!loading && !error && items.length === 0 ? (
          <div className={styles.emptyState}>暂无符合条件的审计记录</div>
        ) : null}
        {items.length ? (
          <div className={styles.auditList}>
            {items.map(item => (
              <article key={item.id}>
                <div>
                  <strong>{item.action}</strong>
                  <span data-result={item.result}>{item.result}</span>
                </div>
                <p>{new Date(item.created_at).toLocaleString('zh-CN')}</p>
                <p>
                  请求 {formatCorrelationID(item.request_id)} · 操作人 #
                  {item.actor_user_id}
                </p>
                {Object.keys(item.metadata).length ? (
                  <dl>
                    {Object.entries(item.metadata).map(([key, value]) => (
                      <div key={key}>
                        <dt>{key}</dt>
                        <dd>{value}</dd>
                      </div>
                    ))}
                  </dl>
                ) : null}
              </article>
            ))}
          </div>
        ) : null}
        <footer className={styles.pagination}>
          <span>
            第 {page} 页 · 共 {total} 条
          </span>
          <button
            aria-label="Sandbox 审计上一页"
            disabled={page <= 1 || loading}
            type="button"
            onClick={() => setPage(value => value - 1)}
          >
            上一页
          </button>
          <button
            aria-label="Sandbox 审计下一页"
            disabled={page * PAGE_SIZE >= total || loading}
            type="button"
            onClick={() => setPage(value => value + 1)}
          >
            下一页
          </button>
        </footer>
      </section>
    </div>
  );
};
