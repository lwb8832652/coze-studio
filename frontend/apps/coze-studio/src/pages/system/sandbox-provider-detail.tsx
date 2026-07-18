/* Copyright 2025 coze-dev Authors */

import { useRef } from 'react';

import type { SandboxProvider } from './sandbox-service';
import {
  getHealthFreshness,
  getHealthLabel,
  getProviderTypeLabel,
  getScopeLabel,
} from './sandbox-view-model';
import styles from './sandbox-management-section.module.less';
import { useSandboxDialogFocus } from './sandbox-dialog-focus';

interface SandboxProviderDetailProps {
  provider: SandboxProvider | null;
  onClose: () => void;
}

const formatTime = (value: string) =>
  value ? new Date(value).toLocaleString('zh-CN', { hour12: false }) : '未检查';

export const SandboxProviderDetail = ({
  provider,
  onClose,
}: SandboxProviderDetailProps) => {
  const closeRef = useRef<HTMLButtonElement>(null);
  const { dialogRef, onDialogKeyDown } = useSandboxDialogFocus({
    initialFocusRef: closeRef,
    onClose,
    open: Boolean(provider),
  });

  if (!provider) {
    return null;
  }
  const freshness = getHealthFreshness(provider.health);

  return (
    <div className={styles.overlay} role="presentation">
      <section
        aria-labelledby="sandbox-provider-detail-title"
        aria-modal="true"
        className={styles.drawer}
        ref={dialogRef}
        role="dialog"
        tabIndex={-1}
        onKeyDown={onDialogKeyDown}
      >
        <header className={styles.drawerHeader}>
          <div>
            <p className={styles.eyebrow}>Provider #{provider.id}</p>
            <h2 id="sandbox-provider-detail-title">{provider.name}</h2>
          </div>
          <button
            ref={closeRef}
            aria-label="关闭 Provider 详情"
            type="button"
            onClick={onClose}
          >
            关闭
          </button>
        </header>
        <dl className={styles.detailGrid}>
          <div>
            <dt>类型</dt>
            <dd>{getProviderTypeLabel(provider.type)}</dd>
          </div>
          <div>
            <dt>状态</dt>
            <dd>{provider.status === 'enabled' ? '已启用' : '已禁用'}</dd>
          </div>
          <div>
            <dt>Endpoint</dt>
            <dd>{provider.endpoint_hint || '未配置'}</dd>
          </div>
          <div>
            <dt>凭据</dt>
            <dd>{provider.credential_fingerprint || '未配置'}</dd>
          </div>
          <div>
            <dt>健康</dt>
            <dd>{getHealthLabel(provider.health.status)}</dd>
          </div>
          <div>
            <dt>健康检查时间</dt>
            <dd>{formatTime(provider.health.checked_at)}</dd>
          </div>
          <div>
            <dt>延迟分组</dt>
            <dd>{provider.health.latency_bucket || '-'}</dd>
          </div>
          <div>
            <dt>适用范围</dt>
            <dd>{provider.scopes.map(getScopeLabel).join('、')}</dd>
          </div>
          <div>
            <dt>版本</dt>
            <dd>v{provider.version}</dd>
          </div>
          <div>
            <dt>最大并发</dt>
            <dd>{provider.policy.max_concurrency}</dd>
          </div>
          <div>
            <dt>超时</dt>
            <dd>{provider.policy.timeout_seconds} 秒</dd>
          </div>
          <div>
            <dt>内存</dt>
            <dd>{provider.policy.memory_limit_mb} MB</dd>
          </div>
          <div>
            <dt>网络</dt>
            <dd>
              {provider.policy.allow_network ? '受 allowlist 限制' : '禁止'}
            </dd>
          </div>
          <div>
            <dt>FFI</dt>
            <dd>{provider.policy.ffi_enabled ? '允许' : '禁止'}</dd>
          </div>
          <div>
            <dt>Node modules</dt>
            <dd>
              {provider.policy.node_modules_mode === 'approved_directory'
                ? provider.policy.node_modules_directory_ref
                : '禁用'}
            </dd>
          </div>
        </dl>
        <div
          className={
            freshness.stale ? styles.warningBanner : styles.healthMessage
          }
          role="status"
        >
          <strong>{freshness.label}</strong>
          <span>
            {provider.health.message ||
              provider.health.reason_code ||
              '无附加健康摘要'}
          </span>
        </div>
      </section>
    </div>
  );
};
