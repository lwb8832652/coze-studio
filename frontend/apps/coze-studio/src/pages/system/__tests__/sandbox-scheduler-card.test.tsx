/* Copyright 2025 coze-dev Authors */

/* eslint-disable @typescript-eslint/require-await -- Browser response doubles keep the async fetch contract. */

import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import { act } from 'react-dom/test-utils';
import { createRoot, type Root } from 'react-dom/client';

import { SandboxSchedulerCard } from '../sandbox-scheduler-card';

globalThis.IS_REACT_ACT_ENVIRONMENT = true;

const settings = {
  total_weight: 2,
  max_outstanding: 32,
  global_queue_depth: 32,
  per_space_queue_depth: 8,
  per_user_queue_depth: 4,
  host_memory_reserve_mb: 1536,
  cancel_grace_seconds: 5,
  health_failure_threshold: 3,
  health_recovery_threshold: 2,
  workloads: {
    agent: {
      weight: 2,
      cpu_limit: 1.25,
      memory_limit_mb: 1536,
      pid_limit: 128,
      queue_timeout_seconds: 600,
      idle_ttl_seconds: 300,
    },
    appdev: {
      weight: 2,
      cpu_limit: 1.25,
      memory_limit_mb: 1536,
      pid_limit: 192,
      queue_timeout_seconds: 1200,
      idle_ttl_seconds: 600,
    },
    mcp_stdio: {
      weight: 1,
      cpu_limit: 0.4,
      memory_limit_mb: 384,
      pid_limit: 64,
      queue_timeout_seconds: 300,
      idle_ttl_seconds: 180,
    },
    plugin: {
      weight: 1,
      cpu_limit: 0.4,
      memory_limit_mb: 384,
      pid_limit: 64,
      queue_timeout_seconds: 300,
      idle_ttl_seconds: 0,
    },
  },
};

describe('SandboxSchedulerCard', () => {
  let container: HTMLDivElement;
  let root: Root;
  beforeEach(() => {
    container = document.createElement('div');
    document.body.appendChild(container);
    root = createRoot(container);
  });
  afterEach(() => {
    act(() => root.unmount());
    container.remove();
    vi.restoreAllMocks();
  });

  it('shows the 2C4G defaults and an unavailable Runner without rendering endpoint or credentials', async () => {
    globalThis.fetch = vi
      .fn()
      .mockResolvedValue({
        ok: true,
        status: 200,
        json: async () => ({
          code: 0,
          msg: '',
          data: { version: 4, settings },
        }),
      })
      .mockResolvedValueOnce({
        ok: true,
        status: 200,
        json: async () => ({
          code: 0,
          msg: '',
          data: { version: 4, settings },
        }),
      })
      .mockResolvedValueOnce({
        ok: true,
        status: 200,
        json: async () => ({
          code: 0,
          msg: '',
          data: {
            available: false,
            desired_config_version: 4,
            applied_config_version: 0,
            reason_code: 'PROVIDER_UNAVAILABLE',
            endpoint: 'must-not-show',
            credential: 'must-not-show',
          },
        }),
      }) as never;
    await act(async () => {
      root.render(<SandboxSchedulerCard />);
      await Promise.resolve();
      await Promise.resolve();
    });
    expect(container.textContent).toContain('2C4G 调度配置');
    expect(container.textContent).toContain('Runner 当前不可用');
    expect(container.textContent).not.toContain('must-not-show');
    expect(container.textContent).not.toContain('凭据');
  });

  it('renders only the aggregate Runner status', async () => {
    globalThis.fetch = vi
      .fn()
      .mockResolvedValueOnce({
        ok: true,
        status: 200,
        json: async () => ({
          code: 0,
          msg: '',
          data: { version: 4, settings },
        }),
      })
      .mockResolvedValueOnce({
        ok: true,
        status: 200,
        json: async () => ({
          code: 0,
          msg: '',
          data: {
            available: true,
            desired_config_version: 4,
            applied_config_version: 4,
            queue_depth: 2,
            queue_by_scope: { agent: 2 },
            active_slots: 1,
            slot_capacity: 2,
            idle_containers: 1,
            active_containers: 1,
            memory_reserve_state: 'available',
            execution_id: 'must-not-show',
            endpoint: 'must-not-show',
          },
        }),
      }) as never;
    await act(async () => {
      root.render(<SandboxSchedulerCard />);
      await Promise.resolve();
      await Promise.resolve();
    });
    expect(container.textContent).toContain('队列 2');
    expect(container.textContent).toContain('运行 1 / 2');
    expect(container.textContent).toContain('内存预留正常');
    expect(container.textContent).not.toContain('must-not-show');
  });
});
