/* Copyright 2025 coze-dev Authors */

import { useEffect, useRef, useState } from 'react';

import { Button, CozInputNumber, Switch } from '@coze-arch/coze-design';

import {
  getSandboxSessionRuntimeStatus,
  getSandboxSessionSettings,
  isSandboxConflict,
  SandboxAPIError,
  updateSandboxSessionSettings,
  type SandboxSessionGenerationState,
  type SandboxSessionRuntimeStatus,
  type SandboxSessionSettings,
} from './sandbox-service';

import styles from './sandbox-session-card.module.less';

const numberFields: Array<{
  key: Exclude<
    keyof SandboxSessionSettings,
    'core_enabled' | 'interactive_enabled' | 'host_shell_enabled'
  >;
  label: string;
  max: number;
  min: number;
}> = [
  { key: 'core_weight', label: 'Core 权重', min: 1, max: 2 },
  { key: 'heavy_weight', label: 'Heavy 权重', min: 1, max: 2 },
  { key: 'per_user_active_limit', label: '单用户活跃上限', min: 1, max: 4096 },
  { key: 'idle_session_limit', label: '空闲 Session 上限', min: 1, max: 4096 },
  { key: 'idle_shell_limit', label: '空闲 Shell 上限', min: 0, max: 4096 },
  {
    key: 'session_idle_ttl_seconds',
    label: 'Session 空闲 TTL（秒）',
    min: 1,
    max: 86400,
  },
  {
    key: 'shell_idle_ttl_seconds',
    label: 'Shell 空闲 TTL（秒）',
    min: 1,
    max: 86400,
  },
  {
    key: 'command_timeout_seconds',
    label: '命令超时（秒）',
    min: 1,
    max: 86400,
  },
  { key: 'cancel_grace_seconds', label: '取消宽限（秒）', min: 1, max: 86400 },
  {
    key: 'workspace_quota_mb',
    label: '工作区限额（MB）',
    min: 1,
    max: 1048576,
  },
];

const validateSettings = (settings: SandboxSessionSettings) => {
  if (settings.interactive_enabled) {
    return 'Phase 1 不允许启用 Interactive Session';
  }
  if (settings.idle_shell_limit > settings.idle_session_limit) {
    return '空闲 Shell 上限不能大于空闲 Session 上限';
  }
  if (settings.shell_idle_ttl_seconds > settings.session_idle_ttl_seconds) {
    return 'Shell 空闲 TTL 不能大于 Session 空闲 TTL';
  }
  if (settings.cancel_grace_seconds > settings.command_timeout_seconds) {
    return '取消宽限不能大于命令超时';
  }
  for (const field of numberFields) {
    const value = settings[field.key];
    if (!Number.isInteger(value) || value < field.min || value > field.max) {
      return `${field.label} 必须是 ${field.min} 到 ${field.max} 的整数`;
    }
  }
  return '';
};

const generationLabels: Record<SandboxSessionGenerationState, string> = {
  disabled: '已关闭',
  unknown: '未知',
  recovering: '恢复中',
  ready: '就绪',
};

const safeRequestMessage = (error: unknown, fallback: string) =>
  error instanceof SandboxAPIError ? error.message : fallback;

// eslint-disable-next-line @coze-arch/max-line-per-function -- Settings and CAS feedback form one cohesive card.
export const SandboxSessionCard = () => {
  const [settings, setSettings] = useState<SandboxSessionSettings | null>(null);
  const [runtime, setRuntime] = useState<SandboxSessionRuntimeStatus | null>(
    null,
  );
  const [version, setVersion] = useState(0);
  const [appliedVersion, setAppliedVersion] = useState(0);
  const [loading, setLoading] = useState(true);
  const [runtimeLoading, setRuntimeLoading] = useState(true);
  const [saving, setSaving] = useState(false);
  const [message, setMessage] = useState('');
  const [runtimeMessage, setRuntimeMessage] = useState('');
  const requestControllerRef = useRef<AbortController | null>(null);

  const load = async (signal: AbortSignal) => {
    setLoading(true);
    setRuntimeLoading(true);
    const [settingsResult, runtimeResult] = await Promise.allSettled([
      getSandboxSessionSettings(signal),
      getSandboxSessionRuntimeStatus(signal),
    ]);
    if (signal.aborted) {
      return;
    }
    if (settingsResult.status === 'fulfilled') {
      const snapshot = settingsResult.value;
      setVersion(snapshot.version);
      setSettings(snapshot.settings);
      setMessage('');
    } else {
      setMessage(
        safeRequestMessage(settingsResult.reason, 'Session 配置加载失败'),
      );
    }
    if (runtimeResult.status === 'fulfilled') {
      setRuntime(runtimeResult.value);
      setAppliedVersion(runtimeResult.value.applied_config_version);
      setRuntimeMessage('');
    } else {
      setRuntimeMessage(
        safeRequestMessage(
          runtimeResult.reason,
          'Remote Runner/Core 运行状态加载失败',
        ),
      );
    }
    setLoading(false);
    setRuntimeLoading(false);
  };

  useEffect(() => {
    const controller = new AbortController();
    requestControllerRef.current = controller;
    void load(controller.signal);
    return () => controller.abort();
  }, []);

  const refresh = () => {
    requestControllerRef.current?.abort();
    const controller = new AbortController();
    requestControllerRef.current = controller;
    void load(controller.signal);
  };

  const save = async () => {
    if (!settings || saving) {
      return;
    }
    const validationMessage = validateSettings(settings);
    if (validationMessage) {
      setMessage(validationMessage);
      return;
    }
    requestControllerRef.current?.abort();
    const controller = new AbortController();
    requestControllerRef.current = controller;
    setSaving(true);
    setMessage('');
    try {
      const result = await updateSandboxSessionSettings(
        version,
        settings,
        controller.signal,
      );
      if (controller.signal.aborted) {
        return;
      }
      setVersion(result.version);
      setSettings(result.settings);
      setAppliedVersion(result.applied_version);
      setMessage(
        result.applied
          ? '期望配置已保存，Remote Runner 已应用 Core 配置版本'
          : `期望配置已保存，Remote Runner 尚未应用 Core 配置版本${
              result.reason_code ? ` · ${result.reason_code}` : ''
            }`,
      );
      setRuntimeLoading(true);
      try {
        const nextRuntime = await getSandboxSessionRuntimeStatus(
          controller.signal,
        );
        if (!controller.signal.aborted) {
          setRuntime(nextRuntime);
          setAppliedVersion(nextRuntime.applied_config_version);
          setRuntimeMessage('');
        }
      } catch (error) {
        if (!controller.signal.aborted) {
          setRuntimeMessage(
            safeRequestMessage(error, 'Remote Runner/Core 运行状态加载失败'),
          );
        }
      } finally {
        if (!controller.signal.aborted) {
          setRuntimeLoading(false);
        }
      }
    } catch (error) {
      if (controller.signal.aborted) {
        return;
      }
      if (isSandboxConflict(error as object)) {
        await load(controller.signal);
        if (!controller.signal.aborted) {
          setMessage('配置版本已变化，已刷新最新配置');
        }
      } else {
        setMessage(
          error instanceof Error ? error.message : 'Session 配置保存失败',
        );
      }
    } finally {
      if (!controller.signal.aborted) {
        setSaving(false);
      }
    }
  };

  const disabled = loading || saving || !settings;

  return (
    <section
      aria-busy={loading || runtimeLoading || saving}
      aria-label="Remote Runner/Core 与 Host Shell 配置"
      className={styles.card}
    >
      <header className={styles.header}>
        <div>
          <h3>Remote Runner/Core</h3>
          <p>
            期望配置 v{version || '-'} · Remote Runner 已应用 v
            {appliedVersion || '-'}
          </p>
        </div>
        <Button
          aria-label="刷新 Core Session 配置和运行状态"
          disabled={loading || saving}
          size="small"
          onClick={refresh}
        >
          刷新
        </Button>
      </header>

      {runtimeMessage ? (
        <div
          aria-live="polite"
          className={styles.runtimeUnavailable}
          role="status"
        >
          <strong>{runtimeMessage}</strong>
          <span>期望配置仍可独立查看和更新。</span>
        </div>
      ) : null}

      {runtime ? (
        <div
          aria-label="Remote Runner/Core 运行聚合"
          className={
            runtime.available ? styles.runtime : styles.runtimeUnavailable
          }
        >
          <div className={styles.runtimeHeader}>
            <div>
              <strong>
                {runtime.available
                  ? 'Remote Runner/Core 运行状态可用'
                  : 'Remote Runner/Core 运行状态不可用'}
              </strong>
              {runtime.reason_code ? <span>{runtime.reason_code}</span> : null}
            </div>
            <span>
              Generation {runtime.runtime_generation || '-'} ·{' '}
              {generationLabels[runtime.generation_state]}
            </span>
          </div>
          <dl className={styles.runtimeGrid}>
            <div>
              <dt>Raw AIO</dt>
              <dd>{runtime.raw_aio_ready ? '就绪' : '未知'}</dd>
            </div>
            <div>
              <dt>Profile</dt>
              <dd>
                Core {runtime.core_enabled ? '开启' : '关闭'} · Interactive{' '}
                {runtime.interactive_enabled ? '开启' : '关闭'}
              </dd>
            </div>
            <div>
              <dt>队列</dt>
              <dd>
                队列 {runtime.queue_depth} · 运行中 {runtime.running}
              </dd>
            </div>
            <div>
              <dt>权重</dt>
              <dd>
                权重 {runtime.used_weight} / {runtime.total_weight}
              </dd>
            </div>
            <div>
              <dt>Session</dt>
              <dd>
                Session 活跃 {runtime.active_sessions} · 空闲{' '}
                {runtime.idle_sessions}
              </dd>
            </div>
            <div>
              <dt>Shell</dt>
              <dd>
                Shell 活跃 {runtime.active_shells} · 空闲 {runtime.idle_shells}
              </dd>
            </div>
          </dl>
          <div
            className={
              !runtime.transport_known
                ? styles.transportUnknown
                : runtime.transport_encrypted
                  ? styles.transportEncrypted
                  : styles.transportWarning
            }
            role="status"
          >
            <strong>
              {!runtime.transport_known
                ? 'Remote Runner 传输状态未知'
                : runtime.transport_encrypted
                  ? 'Remote Runner 传输已加密'
                  : 'Remote Runner 传输未加密'}
            </strong>
            <span>
              {!runtime.transport_known
                ? '尚未从 Remote Runner exact-origin 连接确认传输方式。'
                : runtime.transport_encrypted
                  ? 'Remote Runner HTTPS 管理链路已加密。'
                  : 'Remote Runner HTTP 管理链路未加密，请仅在受控网络中使用。'}
            </span>
          </div>
        </div>
      ) : runtimeLoading ? (
        <div className={styles.loading} role="status">
          正在加载 Remote Runner/Core 运行状态…
        </div>
      ) : null}

      {settings || runtime ? (
        <div
          aria-label="Host Shell 本机 Debug 状态"
          className={styles.hostShellStatus}
          role="status"
        >
          <div className={styles.hostShellHeader}>
            <strong>Host Shell 本机 Debug</strong>
            <span>{runtime?.isolation_level || 'host_debug_unisolated'}</span>
          </div>
          <p>
            期望
            {(runtime?.host_shell_enabled ?? settings?.host_shell_enabled)
              ? '开启'
              : '关闭'}{' '}
            ·{' '}
            {runtime
              ? `本机门禁${runtime.host_shell_available ? '可用' : '不可用'}`
              : '本机门禁状态未知'}
          </p>
          <p className={styles.hostShellWarning}>
            宿主机直接执行，不提供容器、网络、credential
            或恶意命令隔离，仅限可信本机 Debug。
          </p>
        </div>
      ) : null}

      {message ? (
        <div aria-live="polite" className={styles.message} role="status">
          {message}
        </div>
      ) : null}

      {loading && !settings ? (
        <div className={styles.loading} role="status">
          正在加载 Session 配置…
        </div>
      ) : null}

      {settings ? (
        <div className={styles.body}>
          <div className={styles.switches}>
            <div
              className={styles.switchRow}
              data-session-setting="core_enabled"
            >
              <div>
                <strong>Core Profile</strong>
                <span>默认关闭；启用前仍由服务端校验 Runner 就绪状态。</span>
              </div>
              <Switch
                aria-label="启用 Core Session"
                checked={settings.core_enabled}
                disabled={disabled}
                onChange={checked =>
                  setSettings(current =>
                    current ? { ...current, core_enabled: checked } : current,
                  )
                }
              />
            </div>
            <div
              className={styles.switchRow}
              data-session-setting="interactive_enabled"
            >
              <div>
                <strong>Interactive Profile</strong>
                <span>Phase 1 固定关闭。</span>
              </div>
              <Switch
                aria-label="启用 Interactive Session"
                checked={false}
                disabled
              />
            </div>
            <div
              className={styles.switchRow}
              data-session-setting="host_shell_enabled"
            >
              <div>
                <strong>Host Shell</strong>
                <span>仅保存期望值；实际可用性仍受本机 Debug 门禁约束。</span>
              </div>
              <Switch
                aria-label="启用 Host Shell Session"
                checked={settings.host_shell_enabled}
                disabled={disabled}
                onChange={checked =>
                  setSettings(current =>
                    current
                      ? { ...current, host_shell_enabled: checked }
                      : current,
                  )
                }
              />
            </div>
          </div>

          <div className={styles.fields}>
            {numberFields.map(field => (
              <label data-session-setting={field.key} key={field.key}>
                <span>{field.label}</span>
                <CozInputNumber
                  aria-label={field.key}
                  disabled={disabled}
                  max={field.max}
                  min={field.min}
                  precision={0}
                  value={settings[field.key]}
                  onNumberChange={value =>
                    setSettings(current =>
                      current ? { ...current, [field.key]: value } : current,
                    )
                  }
                />
              </label>
            ))}
          </div>

          <footer className={styles.footer}>
            <Button
              aria-label="保存 Core Session 配置"
              color="brand"
              disabled={disabled}
              loading={saving}
              onClick={() => void save()}
            >
              保存完整配置
            </Button>
          </footer>
        </div>
      ) : null}
    </section>
  );
};
