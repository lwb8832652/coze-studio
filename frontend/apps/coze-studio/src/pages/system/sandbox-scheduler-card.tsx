/* Copyright 2025 coze-dev Authors */

import { useEffect, useRef, useState } from 'react';

import {
  getSandboxRuntimeStatus,
  getSandboxSchedulerSettings,
  isSandboxConflict,
  updateSandboxSchedulerSettings,
  type SandboxRuntimeStatus,
  type SandboxSchedulerSettings,
} from './sandbox-service';

import styles from './sandbox-scheduler-card.module.less';

const workloads: Array<[keyof SandboxSchedulerSettings['workloads'], string]> =
  [
    ['agent', 'Agent'],
    ['appdev', '网页应用'],
    ['mcp_stdio', 'MCP'],
    ['plugin', '插件'],
  ];

const validate = (settings: SandboxSchedulerSettings) => {
  if (settings.host_memory_reserve_mb < 1536) {
    return 'host_memory_reserve_mb 不能低于 1536 MB';
  }
  if (settings.per_user_queue_depth > settings.per_space_queue_depth) {
    return 'per_user_queue_depth 不能大于 per_space_queue_depth';
  }
  if (settings.per_space_queue_depth > settings.global_queue_depth) {
    return 'per_space_queue_depth 不能大于 global_queue_depth';
  }
  for (const [scope, workload] of Object.entries(settings.workloads)) {
    if (
      Object.values(workload).some(
        value => !Number.isFinite(value) || value < 0,
      )
    ) {
      return `${scope} 的调度字段必须为有效非负数`;
    }
    if (
      workload.weight < 1 ||
      workload.cpu_limit <= 0 ||
      workload.memory_limit_mb < 1 ||
      workload.pid_limit < 1 ||
      workload.queue_timeout_seconds < 1
    ) {
      return `${scope} 的权重、CPU、内存、PID 和队列超时必须大于 0`;
    }
  }
  return '';
};

// eslint-disable-next-line @coze-arch/max-line-per-function
export const SandboxSchedulerCard = () => {
  const [settings, setSettings] = useState<SandboxSchedulerSettings | null>(
    null,
  );
  const [version, setVersion] = useState(0);
  const [runtime, setRuntime] = useState<SandboxRuntimeStatus | null>(null);
  const [expanded, setExpanded] = useState(false);
  const [saving, setSaving] = useState(false);
  const [message, setMessage] = useState('');
  const requestControllerRef = useRef<AbortController | null>(null);

  const load = async (signal?: AbortSignal) => {
    const [snapshot, status] = await Promise.all([
      getSandboxSchedulerSettings(signal),
      getSandboxRuntimeStatus(signal),
    ]);
    if (signal?.aborted) {
      return;
    }
    setVersion(snapshot.version);
    setSettings(snapshot.settings);
    setRuntime(status);
  };
  useEffect(() => {
    const controller = new AbortController();
    requestControllerRef.current = controller;
    void load(controller.signal).catch(() => {
      if (!controller.signal.aborted) {
        setMessage('调度配置加载失败');
      }
    });
    return () => controller.abort();
  }, []);
  const save = async () => {
    if (!settings || saving) {
      return;
    }
    const validationMessage = validate(settings);
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
      const result = await updateSandboxSchedulerSettings(
        version,
        settings,
        controller.signal,
      );
      if (controller.signal.aborted) {
        return;
      }
      setVersion(result.version);
      setSettings(result.settings);
      setMessage(
        result.applied
          ? '调度配置已保存并应用'
          : '调度配置已保存，等待 Runner 应用',
      );
      setRuntime(await getSandboxRuntimeStatus(controller.signal));
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
        setMessage(error instanceof Error ? error.message : '调度配置保存失败');
      }
    } finally {
      if (!controller.signal.aborted) {
        setSaving(false);
      }
    }
  };
  const updateNumber = (key: keyof SandboxSchedulerSettings, value: string) => {
    if (!settings) {
      return;
    }
    setSettings({ ...settings, [key]: Number(value) });
  };
  const updateWorkload = (
    scope: keyof SandboxSchedulerSettings['workloads'],
    key: keyof SandboxSchedulerSettings['workloads'][typeof scope],
    value: string,
  ) => {
    if (!settings) {
      return;
    }
    setSettings({
      ...settings,
      workloads: {
        ...settings.workloads,
        [scope]: { ...settings.workloads[scope], [key]: Number(value) },
      },
    });
  };

  return (
    <section className={styles.card} aria-label="2C4G 调度配置">
      <header>
        <div>
          <h3>2C4G 调度配置</h3>
          <p>
            配置版本 v{version || '-'} / Runner 已应用 v
            {runtime?.applied_config_version || '-'}
          </p>
        </div>
        <button
          type="button"
          aria-expanded={expanded}
          onClick={() => setExpanded(value => !value)}
        >
          {expanded ? '收起' : '展开'}
        </button>
      </header>
      {runtime && !runtime.available ? (
        <div className={styles.warning} role="status">
          Runner 当前不可用，设置会先保存为期望配置。
        </div>
      ) : null}
      {runtime?.available ? (
        <div className={styles.runtime} aria-label="Runner 运行状态">
          <span>队列 {runtime.queue_depth}</span>
          <span>
            运行 {runtime.active_slots} / {runtime.slot_capacity}
          </span>
          <span>空闲容器 {runtime.idle_containers}</span>
          <span>
            {runtime.memory_reserve_state === 'available'
              ? '内存预留正常'
              : runtime.memory_reserve_state === 'below_watermark'
                ? '内存预留不足'
                : '内存预留未知'}
          </span>
        </div>
      ) : null}
      {message ? (
        <div role="status" className={styles.message}>
          {message}
        </div>
      ) : null}
      {expanded && settings ? (
        <div className={styles.body}>
          <div className={styles.fields}>
            {(
              [
                'total_weight',
                'max_outstanding',
                'global_queue_depth',
                'per_space_queue_depth',
                'per_user_queue_depth',
                'host_memory_reserve_mb',
                'cancel_grace_seconds',
                'health_failure_threshold',
                'health_recovery_threshold',
              ] as const
            ).map(key => (
              <label key={key}>
                {key}
                <input
                  aria-label={key}
                  type="number"
                  min="1"
                  disabled={saving}
                  value={settings[key]}
                  onChange={event => updateNumber(key, event.target.value)}
                />
              </label>
            ))}
          </div>
          <div className={styles.workloads}>
            {workloads.map(([scope, label]) => (
              <article key={scope}>
                <strong>{label}</strong>
                <div className={styles.workloadFields}>
                  {(
                    [
                      'weight',
                      'cpu_limit',
                      'memory_limit_mb',
                      'pid_limit',
                      'queue_timeout_seconds',
                      'idle_ttl_seconds',
                    ] as const
                  ).map(key => (
                    <label key={key}>
                      {key}
                      <input
                        aria-label={`${scope}.${key}`}
                        type="number"
                        min="0"
                        step={key === 'cpu_limit' ? '.001' : '1'}
                        disabled={saving}
                        value={settings.workloads[scope][key]}
                        onChange={event =>
                          updateWorkload(scope, key, event.target.value)
                        }
                      />
                    </label>
                  ))}
                </div>
              </article>
            ))}
          </div>
          <footer>
            <button type="button" disabled={saving} onClick={() => void save()}>
              {saving ? '保存中…' : '保存完整配置'}
            </button>
          </footer>
        </div>
      ) : null}
    </section>
  );
};
