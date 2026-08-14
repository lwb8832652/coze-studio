/* Copyright 2025 coze-dev Authors */

import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import { act, Simulate } from 'react-dom/test-utils';
import { createRoot, type Root } from 'react-dom/client';

globalThis.IS_REACT_ACT_ENVIRONMENT = true;

const mocks = vi.hoisted(() => ({
  getRuntime: vi.fn(),
  getSettings: vi.fn(),
  updateSettings: vi.fn(),
}));

vi.mock('../sandbox-service', async importOriginal => ({
  ...(await importOriginal()),
  getSandboxSessionRuntimeStatus: mocks.getRuntime,
  getSandboxSessionSettings: mocks.getSettings,
  updateSandboxSessionSettings: mocks.updateSettings,
}));

import { SandboxSessionCard } from '../sandbox-session-card';

const settings = {
  core_enabled: false,
  interactive_enabled: false,
  host_shell_enabled: false,
  core_weight: 1,
  heavy_weight: 2,
  per_user_active_limit: 1,
  idle_session_limit: 20,
  idle_shell_limit: 4,
  session_idle_ttl_seconds: 1200,
  shell_idle_ttl_seconds: 300,
  command_timeout_seconds: 600,
  cancel_grace_seconds: 5,
  workspace_quota_mb: 2048,
};

const runtime = {
  available: true,
  desired_config_version: 4,
  applied_config_version: 3,
  runtime_generation: 12,
  core_enabled: false,
  interactive_enabled: false,
  host_shell_enabled: true,
  host_shell_available: false,
  isolation_level: 'host_debug_unisolated' as const,
  raw_aio_ready: false,
  generation_state: 'recovering' as const,
  queue_depth: 3,
  running: 2,
  used_weight: 4,
  total_weight: 8,
  active_sessions: 2,
  idle_sessions: 6,
  active_shells: 1,
  idle_shells: 3,
  transport_known: true,
  transport_encrypted: false,
  reason_code: 'RUNNER_RECOVERING',
  endpoint: 'http://runner.internal:8080',
  endpoint_hint: 'runner.internal',
  sentinel_id: 'sentinel-secret',
  upstream_shell_id: 'shell-secret',
  physical_root: '/private/workspaces/secret',
  env: { SECRET_TOKEN: 'environment-secret' },
  pid: 7319,
  command: 'curl https://secret.internal',
  docker_image: 'ghcr.io/private/image:secret',
  credential: 'secret-token',
  raw_error: 'dial tcp 10.0.0.1:8080: credential=secret',
};

const deferred = <T,>() => {
  let resolve!: (value: T) => void;
  const promise = new Promise<T>(onResolve => {
    resolve = onResolve;
  });
  return { promise, resolve };
};

describe('SandboxSessionCard', () => {
  let container: HTMLDivElement;
  let root: Root;

  beforeEach(() => {
    container = document.createElement('div');
    document.body.appendChild(container);
    root = createRoot(container);
    mocks.getSettings.mockResolvedValue({ version: 4, settings });
    mocks.getRuntime.mockResolvedValue(runtime);
    mocks.updateSettings.mockResolvedValue({
      version: 5,
      settings: { ...settings, core_enabled: true },
      applied: false,
      applied_version: 3,
      reason_code: 'RUNNER_UNAVAILABLE',
    });
  });

  afterEach(() => {
    act(() => root.unmount());
    container.remove();
    vi.clearAllMocks();
  });

  const flush = async () => {
    await Promise.resolve();
    await Promise.resolve();
    await Promise.resolve();
  };

  const render = async () => {
    await act(async () => {
      root.render(<SandboxSessionCard />);
      await flush();
    });
  };

  it('loads settings and runtime in parallel and renders the safe aggregate projection', async () => {
    const settingsRequest = deferred<{
      version: number;
      settings: typeof settings;
    }>();
    const runtimeRequest = deferred<typeof runtime>();
    mocks.getSettings.mockReturnValueOnce(settingsRequest.promise);
    mocks.getRuntime.mockReturnValueOnce(runtimeRequest.promise);

    await act(async () => {
      root.render(<SandboxSessionCard />);
      await flush();
    });

    expect(mocks.getSettings).toHaveBeenCalledWith(expect.any(AbortSignal));
    expect(mocks.getRuntime).toHaveBeenCalledWith(expect.any(AbortSignal));

    await act(async () => {
      settingsRequest.resolve({ version: 4, settings });
      runtimeRequest.resolve(runtime);
      await flush();
    });

    expect(container.textContent).toContain('Remote Runner/Core');
    expect(container.textContent).toContain('期望配置 v4');
    expect(container.textContent).toContain('Remote Runner 已应用 v3');
    expect(container.textContent).toContain('Generation 12');
    expect(container.textContent).toContain('Raw AIO未知');
    expect(container.textContent).toContain('恢复中');
    expect(container.textContent).toContain('队列 3');
    expect(container.textContent).toContain('运行中 2');
    expect(container.textContent).toContain('权重 4 / 8');
    expect(container.textContent).toContain('Session 活跃 2 · 空闲 6');
    expect(container.textContent).toContain('Shell 活跃 1 · 空闲 3');
    expect(container.textContent).toContain('Remote Runner 传输未加密');
    expect(container.textContent).toContain('Remote Runner HTTP 管理链路');
    expect(container.textContent).toContain('Host Shell 本机 Debug');
    expect(container.textContent).toContain('期望开启');
    expect(container.textContent).toContain('本机门禁不可用');
    expect(container.textContent).toContain('host_debug_unisolated');
    expect(container.textContent).toContain(
      '宿主机直接执行，不提供容器、网络、credential 或恶意命令隔离，仅限可信本机 Debug。',
    );
    expect(
      container.querySelector('[aria-label="Host Shell 本机 Debug 状态"]'),
    ).not.toBeNull();
    for (const sensitive of [
      'runner.internal',
      'sentinel-secret',
      'shell-secret',
      '/private/workspaces/secret',
      'environment-secret',
      '7319',
      'curl https://secret.internal',
      'ghcr.io/private/image:secret',
      'secret-token',
      'dial tcp',
    ]) {
      expect(container.textContent).not.toContain(sensitive);
    }
    expect(
      container
        .querySelector<HTMLElement>('[aria-label="启用 Core Session"]')
        ?.getAttribute('aria-checked'),
    ).toBe('false');
    expect(
      container
        .querySelector<HTMLElement>('[aria-label="启用 Interactive Session"]')
        ?.getAttribute('aria-disabled'),
    ).toBe('true');
    expect(container.querySelectorAll('[data-session-setting]').length).toBe(
      13,
    );
  });

  it('refreshes both settings and runtime with a new abortable request', async () => {
    await render();

    const firstSettingsSignal = mocks.getSettings.mock
      .calls[0][0] as AbortSignal;
    const firstRuntimeSignal = mocks.getRuntime.mock.calls[0][0] as AbortSignal;
    await act(async () => {
      Simulate.click(
        container.querySelector<HTMLElement>(
          '[aria-label="刷新 Core Session 配置和运行状态"]',
        )!,
      );
      await flush();
    });

    expect(firstSettingsSignal.aborted).toBe(true);
    expect(firstRuntimeSignal).toBe(firstSettingsSignal);
    expect(mocks.getSettings).toHaveBeenCalledTimes(2);
    expect(mocks.getRuntime).toHaveBeenCalledTimes(2);
    expect(mocks.getSettings.mock.calls[1][0]).toBe(
      mocks.getRuntime.mock.calls[1][0],
    );
  });

  it('renders unknown transport honestly when the runner is unavailable', async () => {
    mocks.getRuntime.mockResolvedValueOnce({
      ...runtime,
      available: false,
      applied_config_version: 0,
      runtime_generation: 0,
      raw_aio_ready: false,
      generation_state: 'unknown',
      transport_known: false,
      transport_encrypted: false,
      host_shell_enabled: false,
      host_shell_available: true,
      reason_code: 'RUNNER_UNAVAILABLE',
    });

    await render();

    expect(container.textContent).toContain('Remote Runner 传输状态未知');
    expect(container.textContent).toContain('本机门禁可用');
    expect(container.textContent).toContain('期望关闭');
    expect(container.textContent).toContain('host_debug_unisolated');
    expect(container.textContent).toContain('仅限可信本机 Debug');
    expect(container.textContent).not.toContain('HTTP Provider');
    expect(container.textContent).not.toContain('HTTPS Provider');
  });

  it('keeps settings usable when runtime refresh fails without rendering the raw error', async () => {
    mocks.getRuntime.mockRejectedValueOnce(
      new Error('dial tcp endpoint=secret.internal credential=secret-token'),
    );

    await render();

    expect(container.textContent).toContain('期望配置 v4');
    expect(container.textContent).toContain(
      'Remote Runner/Core 运行状态加载失败',
    );
    expect(container.textContent).toContain('Host Shell 本机 Debug');
    expect(container.textContent).toContain('本机门禁状态未知');
    expect(container.textContent).toContain('host_debug_unisolated');
    expect(container.textContent).toContain('仅限可信本机 Debug');
    expect(container.textContent).not.toContain('secret.internal');
    expect(container.textContent).not.toContain('secret-token');
  });

  it('sends the full CAS snapshot and reports the applied version and stable reason', async () => {
    await render();

    await act(async () => {
      const coreToggle = container.querySelector<HTMLInputElement>(
        '[aria-label="启用 Core Session"]',
      )!;
      coreToggle.click();
      await flush();
      Simulate.click(
        container.querySelector<HTMLElement>(
          '[aria-label="保存 Core Session 配置"]',
        )!,
      );
      await flush();
    });

    expect(mocks.updateSettings).toHaveBeenCalledWith(
      4,
      { ...settings, core_enabled: true },
      expect.any(AbortSignal),
    );
    expect(container.textContent).toContain('期望配置 v5');
    expect(container.textContent).toContain('Remote Runner 已应用 v3');
    expect(container.textContent).toContain('Remote Runner 尚未应用');
    expect(container.textContent).toContain('RUNNER_UNAVAILABLE');
  });
});
