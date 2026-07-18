/* Copyright 2025 coze-dev Authors */

/* eslint-disable @typescript-eslint/require-await, max-params -- Test doubles preserve async request and callback contracts. */

import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import { act, Simulate } from 'react-dom/test-utils';
import { createRoot, type Root } from 'react-dom/client';

import { SandboxAPIError } from '../sandbox-service';
import { providerFixture } from './sandbox-test-fixtures';

globalThis.IS_REACT_ACT_ENVIRONMENT = true;

const deferred = <T,>() => {
  let resolve!: (value: T) => void;
  let reject!: (error: Error) => void;
  const promise = new Promise<T>((onResolve, onReject) => {
    resolve = onResolve;
    reject = onReject;
  });
  return { promise, reject, resolve };
};

const mocks = vi.hoisted(() => ({
  audit: vi.fn(),
  capabilities: vi.fn(),
  create: vi.fn(),
  defaults: vi.fn(),
  delete: vi.fn(),
  health: vi.fn(),
  list: vi.fn(),
  replaceCredential: vi.fn(),
  setDefault: vi.fn(),
  setEnabled: vi.fn(),
  summary: vi.fn(),
  update: vi.fn(),
}));

vi.mock('../sandbox-service', async importOriginal => ({
  ...(await importOriginal()),
  createSandboxProvider: mocks.create,
  deleteSandboxProvider: mocks.delete,
  getSandboxCapabilities: mocks.capabilities,
  getSandboxProviderSummary: mocks.summary,
  healthCheckSandboxProvider: mocks.health,
  listSandboxAuditEvents: mocks.audit,
  listSandboxProviderDefaults: mocks.defaults,
  listSandboxProviders: mocks.list,
  replaceSandboxProviderCredential: mocks.replaceCredential,
  setSandboxProviderDefault: mocks.setDefault,
  setSandboxProviderEnabled: mocks.setEnabled,
  updateSandboxProvider: mocks.update,
}));

import { SandboxManagementSection } from '../sandbox-management-section';

const defaultItems = [
  {
    scope: 'agent' as const,
    configured: true,
    provider_id: 17,
    provider_name: 'Primary',
    provider_type: 'remote_http' as const,
    provider_status: 'enabled' as const,
    version: 3,
    provider_version: 7,
    updated_at: '2026-07-16T08:00:00Z',
  },
  {
    scope: 'mcp_stdio' as const,
    configured: false,
    provider_id: null,
    provider_name: '',
    provider_type: '' as const,
    provider_status: '' as const,
    version: 0,
    provider_version: 0,
    updated_at: null,
  },
  {
    scope: 'appdev' as const,
    configured: true,
    provider_id: 17,
    provider_name: 'Primary',
    provider_type: 'remote_http' as const,
    provider_status: 'enabled' as const,
    version: 5,
    provider_version: 7,
    updated_at: '2026-07-16T08:00:00Z',
  },
];

const capabilities = {
  control_plane: {
    available: true,
    reason_code: 'AVAILABLE',
    message: 'control plane ready',
  },
  local_debug: {
    available: true,
    reason_code: 'AVAILABLE',
    message: 'local debug ready',
  },
};

describe('SandboxManagementSection', () => {
  let container: HTMLDivElement;
  let root: Root;

  beforeEach(() => {
    container = document.createElement('div');
    document.body.appendChild(container);
    root = createRoot(container);
    mocks.list.mockResolvedValue({ items: [providerFixture], total: 41 });
    mocks.defaults.mockResolvedValue({ items: defaultItems });
    mocks.summary.mockResolvedValue({ total: 12, enabled: 8, unhealthy: 2 });
    mocks.capabilities.mockResolvedValue(capabilities);
    mocks.audit.mockResolvedValue({ items: [], total: 0 });
    mocks.create.mockResolvedValue(providerFixture);
    mocks.update.mockResolvedValue({ ...providerFixture, version: 8 });
    mocks.replaceCredential.mockResolvedValue({
      ...providerFixture,
      version: 9,
    });
    mocks.health.mockResolvedValue({ ...providerFixture, version: 8 });
    mocks.setEnabled.mockResolvedValue({
      ...providerFixture,
      status: 'disabled',
      version: 8,
    });
    mocks.setDefault.mockResolvedValue({
      scope: 'agent',
      provider_id: 17,
      version: 4,
      updated_at: '2026-07-16T08:00:00Z',
    });
    mocks.delete.mockResolvedValue({ provider_id: 17, version: 8 });
    vi.spyOn(window, 'confirm').mockReturnValue(true);
  });

  afterEach(() => {
    act(() => root.unmount());
    container.remove();
    vi.restoreAllMocks();
    vi.clearAllMocks();
  });

  const flush = async () => {
    await Promise.resolve();
    await Promise.resolve();
    await Promise.resolve();
  };

  const render = async () => {
    await act(async () => {
      root.render(<SandboxManagementSection />);
      await flush();
    });
  };

  const click = async (label: string) => {
    await act(async () => {
      Simulate.click(
        container.querySelector<HTMLButtonElement>(
          `button[aria-label="${label}"]`,
        )!,
      );
      await flush();
    });
  };

  it('initializes from real list/defaults/summary/capability projections', async () => {
    await render();

    expect(mocks.list).toHaveBeenCalledTimes(1);
    expect(mocks.defaults).toHaveBeenCalledTimes(1);
    expect(mocks.summary).toHaveBeenCalledTimes(1);
    expect(mocks.capabilities).toHaveBeenCalledTimes(1);
    expect(container.textContent).toContain('12');
    expect(container.textContent).toContain('8');
    expect(container.textContent).toContain('2');
    expect(container.textContent).toContain('仅统计 unhealthy');
    expect(container.textContent).not.toContain('degraded / unhealthy');
    expect(container.textContent).toContain('control plane ready');
    expect(container.textContent).toContain('Primary');
    expect(container.textContent).toContain('bounded health message');
    expect(container.textContent).not.toContain('当前查询结果');
    expect(container.textContent).not.toContain('本页 Provider');
    expect(container.innerHTML).not.toContain('must-never-survive');
  });

  it('keeps an unavailable control-plane capability when provider projections fail', async () => {
    mocks.capabilities.mockResolvedValueOnce({
      control_plane: {
        available: false,
        reason_code: 'CONTROL_PLANE_DISABLED',
        message: 'Sandbox control plane is disabled',
      },
      local_debug: {
        available: false,
        reason_code: 'LOCAL_DEBUG_DISABLED',
        message: 'Local debug is disabled',
      },
    });
    mocks.list.mockRejectedValueOnce(
      new SandboxAPIError(503, 'SANDBOX_UNAVAILABLE'),
    );

    await render();

    expect(container.textContent).toContain('控制面不可用');
    expect(container.textContent).toContain(
      'Sandbox control plane is disabled',
    );
    expect(
      container.querySelector<HTMLButtonElement>(
        'button[aria-label="创建 Sandbox Provider"]',
      )?.disabled,
    ).toBe(true);
    expect(container.textContent).toContain('Sandbox 控制面当前不可用');
  });

  it('applies health filtering and paginates independently of global summary', async () => {
    await render();
    await act(async () => {
      Simulate.change(
        container.querySelector<HTMLSelectElement>(
          'select[aria-label="按健康状态筛选"]',
        )!,
        {
          target: { value: 'unhealthy' },
        },
      );
    });
    await act(async () => {
      Simulate.click(
        container.querySelector<HTMLButtonElement>(
          'button[aria-label="应用 Sandbox 筛选"]',
        )!,
      );
      await flush();
    });
    expect(mocks.list).toHaveBeenLastCalledWith(
      expect.objectContaining({ health: 'unhealthy', offset: 0 }),
      expect.any(AbortSignal),
    );

    await click('Sandbox Provider 下一页');
    expect(mocks.list).toHaveBeenLastCalledWith(
      expect.objectContaining({ health: 'unhealthy', offset: 20 }),
      expect.any(AbortSignal),
    );
    expect(mocks.summary).toHaveBeenCalledTimes(3);
  });

  it('uses default CAS version 3 separately from provider version 7 and refreshes projections', async () => {
    await render();
    await click('将 Primary 设为 Agent 默认 Provider');

    expect(mocks.setDefault).toHaveBeenCalledWith(
      17,
      7,
      'agent',
      3,
      expect.anything(),
    );
    expect(mocks.list).toHaveBeenCalledTimes(2);
    expect(mocks.defaults).toHaveBeenCalledTimes(2);
    expect(mocks.summary).toHaveBeenCalledTimes(2);
    expect(mocks.capabilities).toHaveBeenCalledTimes(1);
  });

  it('creates with the real form and refreshes list/defaults/summary', async () => {
    await render();
    await click('创建 Sandbox Provider');
    await act(async () => {
      const set = (label: string, value: string) => {
        const input = container.querySelector<HTMLInputElement>(
          `input[aria-label="${label}"]`,
        )!;
        input.value = value;
        Simulate.change(input);
      };
      set('Provider 名称', 'Secondary');
      set('Provider Endpoint', 'https://runner.example.test');
      set('Provider 凭据', 'create-secret');
    });
    await act(async () => {
      Simulate.click(
        container.querySelector<HTMLButtonElement>(
          'button[aria-label="创建 Provider"]',
        )!,
      );
      await flush();
    });

    expect(mocks.create).toHaveBeenCalledWith(
      expect.objectContaining({
        name: 'Secondary',
        type: 'remote_http',
        credential: 'create-secret',
        policy: expect.objectContaining({
          allowed_env_names: [],
          virtual_read_prefixes: [],
          virtual_write_prefixes: [],
          allowed_executables: [],
          ffi_enabled: false,
          node_modules_mode: 'disabled',
        }),
      }),
      expect.any(AbortSignal),
    );
    expect(mocks.defaults).toHaveBeenCalledTimes(2);
    expect(mocks.summary).toHaveBeenCalledTimes(2);
  });

  it('recovers from partial credential replacement success without leaving a stale form', async () => {
    mocks.replaceCredential.mockRejectedValueOnce(
      new Error('credential transport failed'),
    );
    await render();
    await click('编辑 Primary');
    await act(async () => {
      Simulate.change(
        container.querySelector<HTMLInputElement>(
          'input[aria-label="替换 Provider 凭据"]',
        )!,
        {
          target: { checked: true },
        },
      );
    });
    await act(async () => {
      const credential = container.querySelector<HTMLInputElement>(
        'input[aria-label="新的 Provider 凭据"]',
      )!;
      credential.value = 'replacement-secret';
      Simulate.change(credential);
    });
    await act(async () => {
      Simulate.click(
        container.querySelector<HTMLButtonElement>(
          'button[aria-label="保存 Provider"]',
        )!,
      );
      await flush();
    });

    expect(mocks.update).toHaveBeenCalledWith(
      17,
      expect.objectContaining({ expected_version: 7 }),
      expect.any(AbortSignal),
    );
    expect(mocks.replaceCredential).toHaveBeenCalledWith(
      17,
      8,
      'replacement-secret',
      expect.any(AbortSignal),
    );
    expect(container.querySelector('[role="dialog"]')).toBeNull();
    expect(container.textContent).toContain('基础配置已保存，但凭据替换失败');
    expect(mocks.list).toHaveBeenCalledTimes(2);
    expect(mocks.defaults).toHaveBeenCalledTimes(2);
    expect(mocks.summary).toHaveBeenCalledTimes(2);
  });

  it('refreshes and offers recovery for ordinary 409 conflicts', async () => {
    mocks.health.mockRejectedValueOnce(
      new SandboxAPIError(409, 'SANDBOX_VERSION_CONFLICT'),
    );
    await render();
    await click('检查 Primary 健康状态');

    expect(container.textContent).toContain('配置已被其他管理员更新');
    expect(container.textContent).toContain('刷新最新数据');
    expect(mocks.list).toHaveBeenCalledTimes(2);
    expect(mocks.defaults).toHaveBeenCalledTimes(2);
    expect(mocks.summary).toHaveBeenCalledTimes(2);
  });

  it('confirms health/status/delete operations and refreshes all affected projections', async () => {
    await render();
    await click('检查 Primary 健康状态');
    await click('禁用并排空 Primary');
    await click('删除 Primary');

    expect(mocks.health).toHaveBeenCalledWith(17, 7, expect.anything());
    expect(mocks.setEnabled).toHaveBeenCalledWith(
      17,
      7,
      false,
      expect.anything(),
    );
    expect(mocks.delete).toHaveBeenCalledWith(17, 7, expect.anything());
    expect(window.confirm).toHaveBeenCalled();
    expect(mocks.list).toHaveBeenCalledTimes(4);
    expect(mocks.defaults).toHaveBeenCalledTimes(4);
    expect(mocks.summary).toHaveBeenCalledTimes(4);
  });

  it('renders loading/error/retry/empty and safe permission failures', async () => {
    mocks.defaults.mockRejectedValueOnce(
      new SandboxAPIError(403, 'SANDBOX_POLICY_DENIED'),
    );
    await render();
    expect(container.textContent).toContain('当前操作被 Sandbox 安全策略拒绝');
    await click('重试加载 Sandbox Provider');
    expect(mocks.defaults).toHaveBeenCalledTimes(2);

    mocks.list.mockResolvedValueOnce({ items: [], total: 0 });
    await click('刷新 Sandbox Provider');
    expect(container.textContent).toContain('暂无 Sandbox Provider');
  });

  it('ignores an older list response after rapid filter changes', async () => {
    await render();
    const older = deferred<{
      items: (typeof providerFixture)[];
      total: number;
    }>();
    const latest = deferred<{
      items: (typeof providerFixture)[];
      total: number;
    }>();
    const signals: AbortSignal[] = [];
    mocks.list
      .mockImplementationOnce((_filters, signal) => {
        signals.push(signal);
        return older.promise;
      })
      .mockImplementationOnce((_filters, signal) => {
        signals.push(signal);
        return latest.promise;
      });

    const health = container.querySelector<HTMLSelectElement>(
      'select[aria-label="按健康状态筛选"]',
    )!;
    await act(async () => {
      Simulate.change(health, { target: { value: 'unhealthy' } });
      Simulate.click(
        container.querySelector<HTMLButtonElement>(
          'button[aria-label="应用 Sandbox 筛选"]',
        )!,
      );
      await flush();
      Simulate.change(health, { target: { value: 'healthy' } });
      Simulate.click(
        container.querySelector<HTMLButtonElement>(
          'button[aria-label="应用 Sandbox 筛选"]',
        )!,
      );
      await flush();
    });

    await act(async () => {
      latest.resolve({
        items: [{ ...providerFixture, id: 19, name: 'Latest Provider' }],
        total: 1,
      });
      await flush();
    });
    await act(async () => {
      older.resolve({
        items: [{ ...providerFixture, id: 18, name: 'Older Provider' }],
        total: 1,
      });
      await flush();
    });

    expect(signals[0].aborted).toBe(true);
    expect(container.textContent).toContain('Latest Provider');
    expect(container.textContent).not.toContain('Older Provider');
  });

  it('aborts projection reads and ignores their completion after unmount', async () => {
    const pending = deferred<{
      items: (typeof providerFixture)[];
      total: number;
    }>();
    let signal: AbortSignal | undefined;
    mocks.list.mockImplementationOnce((_filters, requestSignal) => {
      signal = requestSignal;
      return pending.promise;
    });
    await act(async () => {
      root.render(<SandboxManagementSection />);
      await flush();
    });
    act(() => root.unmount());
    expect(signal?.aborted).toBe(true);
    await act(async () => {
      pending.resolve({
        items: [{ ...providerFixture, name: 'Late Provider' }],
        total: 1,
      });
      await flush();
    });
    root = createRoot(container);
    expect(container.textContent).not.toContain('Late Provider');
  });

  it('separates a successful mutation from refresh failure and clears pending', async () => {
    await render();
    mocks.list.mockRejectedValueOnce(new Error('projection offline'));
    await click('检查 Primary 健康状态');

    expect(container.textContent).toContain('操作已成功，但刷新失败，可重试');
    expect(container.querySelector('section')?.getAttribute('aria-busy')).toBe(
      'false',
    );
    expect(
      container.querySelector<HTMLButtonElement>(
        'button[aria-label="编辑 Primary"]',
      )?.disabled,
    ).toBe(true);

    await click('重试刷新 Sandbox 投影');
    expect(container.textContent).toContain('最新 Sandbox 数据已刷新');
    expect(
      container.querySelector<HTMLButtonElement>(
        'button[aria-label="编辑 Primary"]',
      )?.disabled,
    ).toBe(false);
  });

  it.each(['resolve', 'reject'] as const)(
    'does not continue a partial credential mutation after unmount when it %s',
    async outcome => {
      const pendingCredential = deferred<typeof providerFixture>();
      let mutationSignal: AbortSignal | undefined;
      const consoleError = vi
        .spyOn(console, 'error')
        .mockImplementation(() => undefined);
      mocks.replaceCredential.mockImplementationOnce(
        (_providerID, _version, _credential, signal: AbortSignal) => {
          mutationSignal = signal;
          return pendingCredential.promise;
        },
      );

      await render();
      await click('编辑 Primary');
      await act(async () => {
        Simulate.change(
          container.querySelector<HTMLInputElement>(
            'input[aria-label="替换 Provider 凭据"]',
          )!,
          { target: { checked: true } },
        );
        await flush();
      });
      await act(async () => {
        const credential = container.querySelector<HTMLInputElement>(
          'input[aria-label="新的 Provider 凭据"]',
        )!;
        credential.value = 'replacement-secret';
        Simulate.change(credential);
        await flush();
      });
      await act(async () => {
        Simulate.click(
          container.querySelector<HTMLButtonElement>(
            'button[aria-label="保存 Provider"]',
          )!,
        );
        await flush();
      });

      expect(mocks.update).toHaveBeenCalledTimes(1);
      expect(mocks.replaceCredential).toHaveBeenCalledTimes(1);
      act(() => root.unmount());
      expect(mutationSignal?.aborted).toBe(true);

      await act(async () => {
        if (outcome === 'resolve') {
          pendingCredential.resolve({ ...providerFixture, version: 9 });
        } else {
          pendingCredential.reject(new Error('late credential failure'));
        }
        await flush();
      });

      expect(mocks.list).toHaveBeenCalledTimes(1);
      expect(mocks.defaults).toHaveBeenCalledTimes(1);
      expect(mocks.summary).toHaveBeenCalledTimes(1);
      expect(consoleError.mock.calls.flat().join(' ')).not.toMatch(
        /state update|unmounted component/i,
      );
      root = createRoot(container);
    },
  );
});
