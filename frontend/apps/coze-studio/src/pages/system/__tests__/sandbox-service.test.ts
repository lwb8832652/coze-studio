/* Copyright 2025 coze-dev Authors */

/* eslint-disable @typescript-eslint/require-await -- Response test doubles preserve async browser contracts. */

import { afterEach, describe, expect, it, vi } from 'vitest';

import {
  clearSandboxProviderCredential,
  createSandboxProvider,
  deleteSandboxProvider,
  getSandboxCapabilities,
  getSandboxProviderSummary,
  getSandboxRuntimeStatus,
  getSandboxSchedulerSettings,
  healthCheckSandboxProvider,
  isSandboxConflict,
  isSandboxPermissionError,
  listSandboxAuditEvents,
  listSandboxProviderDefaults,
  listSandboxProviders,
  replaceSandboxProviderCredential,
  SandboxAPIError,
  setSandboxProviderDefault,
  setSandboxProviderEnabled,
  updateSandboxSchedulerSettings,
  updateSandboxProvider,
} from '../sandbox-service';
import { fullPolicy, providerFixture } from './sandbox-test-fixtures';

const ok = (data: object) => ({
  ok: true,
  status: 200,
  json: async () => ({ code: 0, msg: '', data }),
});

describe('sandbox service', () => {
  afterEach(() => vi.restoreAllMocks());

  it('loads and saves the complete scheduler snapshot without surfacing runner secrets', async () => {
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
    const fetchMock = vi
      .fn()
      .mockResolvedValueOnce(ok({ version: 4, settings }))
      .mockResolvedValueOnce(
        ok({
          version: 5,
          settings,
          applied: false,
          reason_code: 'PROVIDER_UNAVAILABLE',
        }),
      )
      .mockResolvedValueOnce(
        ok({
          available: false,
          desired_config_version: 5,
          applied_config_version: 0,
          reason_code: 'PROVIDER_UNAVAILABLE',
          endpoint: 'must-not-surface',
          credential: 'must-not-surface',
        }),
      );
    globalThis.fetch = fetchMock as never;

    expect(await getSandboxSchedulerSettings()).toEqual({
      version: 4,
      settings,
    });
    expect(await updateSandboxSchedulerSettings(4, settings)).toMatchObject({
      version: 5,
      applied: false,
    });
    const runtime = await getSandboxRuntimeStatus();
    expect(fetchMock).toHaveBeenNthCalledWith(
      1,
      '/api/admin/sandboxes/scheduler-settings',
      expect.objectContaining({ method: 'GET' }),
    );
    expect(fetchMock).toHaveBeenNthCalledWith(
      2,
      '/api/admin/sandboxes/scheduler-settings',
      expect.objectContaining({
        method: 'PUT',
        body: JSON.stringify({ expected_version: 4, settings }),
      }),
    );
    expect(fetchMock).toHaveBeenNthCalledWith(
      3,
      '/api/admin/sandboxes/runtime-status',
      expect.objectContaining({ method: 'GET' }),
    );
    expect(runtime).not.toHaveProperty('endpoint');
    expect(runtime).not.toHaveProperty('credential');
  });

  it('loads list filters plus server-owned defaults, summary and capabilities', async () => {
    const fetchMock = vi
      .fn()
      .mockResolvedValueOnce(ok({ items: [providerFixture], total: 41 }))
      .mockResolvedValueOnce(
        ok({
          items: [
            {
              scope: 'agent',
              configured: true,
              provider_id: 17,
              provider_name: 'Primary',
              provider_type: 'remote_http',
              provider_status: 'enabled',
              version: 3,
              provider_version: 7,
              updated_at: '2026-07-16T08:00:00Z',
            },
          ],
        }),
      )
      .mockResolvedValueOnce(ok({ total: 12, enabled: 8, unhealthy: 2 }))
      .mockResolvedValueOnce(
        ok({
          control_plane: {
            available: true,
            reason_code: 'AVAILABLE',
            message: 'available',
          },
          local_debug: {
            available: true,
            reason_code: 'AVAILABLE',
            message: 'debug available',
          },
        }),
      );
    globalThis.fetch = fetchMock as never;

    const controller = new AbortController();
    const list = await listSandboxProviders(
      {
        health: 'unhealthy',
        limit: 20,
        offset: 20,
        scope: 'agent',
        status: 'enabled',
        type: 'remote_http',
      },
      controller.signal,
    );
    const defaults = await listSandboxProviderDefaults(controller.signal);
    const summary = await getSandboxProviderSummary(controller.signal);
    const capabilities = await getSandboxCapabilities(controller.signal);

    expect(fetchMock).toHaveBeenNthCalledWith(
      1,
      '/api/admin/sandboxes?type=remote_http&status=enabled&health=unhealthy&scope=agent&offset=20&limit=20',
      expect.objectContaining({
        credentials: 'include',
        signal: controller.signal,
      }),
    );
    expect(fetchMock).toHaveBeenNthCalledWith(
      2,
      '/api/admin/sandboxes/defaults',
      expect.objectContaining({ method: 'GET', signal: controller.signal }),
    );
    expect(fetchMock).toHaveBeenNthCalledWith(
      3,
      '/api/admin/sandboxes/summary',
      expect.objectContaining({ method: 'GET', signal: controller.signal }),
    );
    expect(fetchMock).toHaveBeenNthCalledWith(
      4,
      '/api/admin/sandboxes/capabilities',
      expect.objectContaining({ method: 'GET', signal: controller.signal }),
    );
    expect(list.items[0]).not.toHaveProperty('credential');
    expect(list.items[0]).not.toHaveProperty('endpoint');
    expect(list.items[0].health.message).toBe('bounded health message');
    expect(defaults.items[0]).toMatchObject({
      version: 3,
      provider_version: 7,
    });
    expect(summary).toEqual({ total: 12, enabled: 8, unhealthy: 2 });
    expect(capabilities.local_debug.available).toBe(true);
  });

  it('preserves the plugin scope in filters and provider projections', async () => {
    globalThis.fetch = vi.fn().mockResolvedValue(
      ok({
        items: [
          {
            ...providerFixture,
            scopes: ['plugin'],
            health: {
              ...providerFixture.health,
              capabilities: ['plugin'],
            },
          },
        ],
        total: 1,
      }),
    ) as never;

    const result = await listSandboxProviders({
      limit: 20,
      offset: 0,
      scope: 'plugin',
    });

    expect(globalThis.fetch).toHaveBeenCalledWith(
      '/api/admin/sandboxes?scope=plugin&offset=0&limit=20',
      expect.objectContaining({ method: 'GET' }),
    );
    expect(result.items[0]).toMatchObject({
      scopes: ['plugin'],
      health: { capabilities: ['plugin'] },
    });
  });

  it('adds a unique secure request ID to every mutation and keeps CAS versions distinct', async () => {
    const fetchMock = vi.fn().mockResolvedValue(ok(providerFixture));
    globalThis.fetch = fetchMock as never;

    await createSandboxProvider({
      name: 'Primary',
      type: 'remote_http',
      endpoint: 'https://runner.example.test',
      credential: 'create-secret',
      scopes: ['agent'],
      policy: fullPolicy,
    });
    await updateSandboxProvider(17, {
      expected_version: 7,
      name: 'Primary v2',
      scopes: ['agent'],
      policy: fullPolicy,
      credential: { mode: 'keep', value: '' },
    });
    await replaceSandboxProviderCredential(17, 8, 'replace-secret');
    await clearSandboxProviderCredential(17, 9);
    await setSandboxProviderEnabled(17, 10, false);
    await healthCheckSandboxProvider(17, 11);
    await setSandboxProviderDefault(17, 12, 'agent', 3);
    await deleteSandboxProvider(17, 13);

    expect(fetchMock).toHaveBeenNthCalledWith(
      7,
      '/api/admin/sandboxes/17/defaults',
      expect.objectContaining({
        body: JSON.stringify({
          expected_version: 12,
          default_expected_version: 3,
          scope: 'agent',
        }),
      }),
    );
    const requestIDs = fetchMock.mock.calls.map(([, init]) =>
      new Headers((init as RequestInit).headers).get('X-Request-ID'),
    );
    expect(requestIDs).toHaveLength(8);
    expect(requestIDs.every(value => Boolean(value))).toBe(true);
    expect(new Set(requestIDs).size).toBe(requestIDs.length);
    expect(
      requestIDs.every(value => /^[0-9a-f-]{36}$/i.test(value || '')),
    ).toBe(true);
  });

  it('maps 409, 401, 403 and safe server field errors without raw response leakage', async () => {
    const responses = [
      { status: 409, error_code: 'SANDBOX_VERSION_CONFLICT' },
      { status: 401, error_code: 'AUTHENTICATION_REQUIRED' },
      { status: 403, error_code: 'SANDBOX_POLICY_DENIED' },
      {
        status: 400,
        error_code: 'SANDBOX_CONFIGURATION_INVALID',
        field_errors: {
          request: '请求参数无效',
          credential: '凭据配置无效',
          scopes: '所选作用域不受支持',
          type: '当前环境不支持本地调试 Sandbox',
          name: 'must-drop-name',
          'policy.allowed_env_names': 'must-drop-policy',
          raw_provider_body: 'must-drop',
        },
      },
    ];
    globalThis.fetch = vi.fn().mockImplementation(async () => {
      const response = responses.shift()!;
      return {
        ok: false,
        status: response.status,
        json: async () => ({ ...response, raw_provider_body: 'secret-body' }),
      };
    }) as never;

    const conflict = await healthCheckSandboxProvider(17, 1).catch(
      error => error,
    );
    const unauthenticated = await healthCheckSandboxProvider(17, 1).catch(
      error => error,
    );
    const forbidden = await healthCheckSandboxProvider(17, 1).catch(
      error => error,
    );
    const invalid = await healthCheckSandboxProvider(17, 1).catch(
      error => error,
    );

    expect(isSandboxConflict(conflict)).toBe(true);
    expect(isSandboxPermissionError(unauthenticated)).toBe(true);
    expect(isSandboxPermissionError(forbidden)).toBe(true);
    expect(invalid).toBeInstanceOf(SandboxAPIError);
    expect((invalid as SandboxAPIError).fieldErrors).toEqual({
      request: '请求参数无效',
      credential: '凭据配置无效',
      scopes: '所选作用域不受支持',
      type: '当前环境不支持本地调试 Sandbox',
    });
    expect(String(invalid)).not.toContain('secret-body');
  });

  it('keeps only the canonical bounded audit metadata and paginates', async () => {
    globalThis.fetch = vi.fn().mockResolvedValue(
      ok({
        items: [
          {
            id: 31,
            provider_id: 17,
            actor_user_id: 42,
            action: 'health_check',
            result: 'success',
            request_id: 'request-1',
            metadata: {
              scope: 'agent',
              changed_fields: 'health',
              previous_status: 'disabled',
              new_status: 'enabled',
              health_code: 'AVAILABLE',
              version: '8',
              raw_body: 'must-drop',
              status: 'must-drop',
            },
            created_at: '2026-07-16T08:00:00Z',
          },
        ],
        total: 21,
      }),
    ) as never;

    const result = await listSandboxAuditEvents(17, {
      action: 'health_check',
      limit: 20,
      offset: 20,
      result: 'success',
    });

    expect(result.items[0].metadata).toEqual({
      scope: 'agent',
      changed_fields: 'health',
      previous_status: 'disabled',
      new_status: 'enabled',
      health_code: 'AVAILABLE',
      version: '8',
    });
    expect(result.total).toBe(21);
    expect(globalThis.fetch).toHaveBeenCalledWith(
      '/api/admin/sandboxes/17/audit-events?action=health_check&result=success&offset=20&limit=20',
      expect.objectContaining({ method: 'GET' }),
    );
  });
});
