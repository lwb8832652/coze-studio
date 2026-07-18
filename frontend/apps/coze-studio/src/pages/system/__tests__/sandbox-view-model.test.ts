/* Copyright 2025 coze-dev Authors */

import { describe, expect, it } from 'vitest';

import {
  SANDBOX_SCOPE_OPTIONS,
  createSandboxViewState,
  filterSandboxScopesForProviderType,
  getHealthFreshness,
  getLocalDebugAvailability,
  getSandboxScopeOptions,
  getScopeLabel,
  sandboxViewReducer,
  validateSandboxRuntimePolicy,
} from '../sandbox-view-model';
import { fullPolicy, providerFixture } from './sandbox-test-fixtures';

describe('sandbox view model', () => {
  it('models loading, empty, error, mutation and conflict states', () => {
    const initial = createSandboxViewState();
    const ready = sandboxViewReducer(initial, {
      type: 'load_succeeded',
      items: [providerFixture],
      total: 1,
    });
    const mutating = sandboxViewReducer(ready, {
      type: 'mutation_started',
      action: 'health',
      providerID: 17,
    });
    const conflict = sandboxViewReducer(mutating, {
      type: 'mutation_failed',
      conflict: true,
      message: '配置已变化',
    });

    expect(initial.phase).toBe('loading');
    expect(ready.phase).toBe('ready');
    expect(mutating.pending).toEqual({ action: 'health', providerID: 17 });
    expect(conflict.conflict).toBe(true);
    expect(
      sandboxViewReducer(initial, {
        type: 'load_failed',
        message: '加载失败',
      }).phase,
    ).toBe('error');
    expect(
      sandboxViewReducer(initial, {
        type: 'load_succeeded',
        items: [],
        total: 0,
      }).phase,
    ).toBe('empty');
  });

  it('uses server capability instead of frontend environment guesses', () => {
    expect(
      getLocalDebugAvailability({
        control_plane: {
          available: true,
          reason_code: 'AVAILABLE',
          message: 'available',
        },
        local_debug: {
          available: true,
          reason_code: 'AVAILABLE',
          message: 'local enabled',
        },
      }),
    ).toEqual({ enabled: true, reason: 'local enabled' });
    expect(
      getLocalDebugAvailability({
        control_plane: {
          available: true,
          reason_code: 'AVAILABLE',
          message: 'available',
        },
        local_debug: {
          available: false,
          reason_code: 'LOCAL_DEBUG_UNAVAILABLE',
          message: 'local disabled',
        },
      }),
    ).toEqual({ enabled: false, reason: 'local disabled' });
  });

  it('uses the five-minute runtime health threshold for stale warnings', () => {
    const health = {
      ...providerFixture.health,
      checked_at: '2026-07-16T08:00:00Z',
    };
    expect(
      getHealthFreshness(health, new Date('2026-07-16T08:04:59Z')),
    ).toMatchObject({ stale: false });
    expect(
      getHealthFreshness(health, new Date('2026-07-16T08:05:01Z')),
    ).toMatchObject({ stale: true, label: '健康结果已过期' });
    expect(
      getHealthFreshness({
        ...providerFixture.health,
        status: 'unknown',
        checked_at: '',
      }),
    ).toMatchObject({ stale: true, label: '尚未完成健康检查' });
  });

  it('validates the complete bounded policy and rejects values, host paths, traversal and commands', () => {
    expect(validateSandboxRuntimePolicy(fullPolicy)).toEqual({});
    expect(
      validateSandboxRuntimePolicy({
        ...fullPolicy,
        allowed_env_names: ['PATH=secret'],
      }),
    ).toHaveProperty('allowed_env_names');
    expect(
      validateSandboxRuntimePolicy({
        ...fullPolicy,
        virtual_read_prefixes: ['/etc'],
      }),
    ).toHaveProperty('virtual_read_prefixes');
    expect(
      validateSandboxRuntimePolicy({
        ...fullPolicy,
        virtual_write_prefixes: ['workspace/../etc'],
      }),
    ).toHaveProperty('virtual_write_prefixes');
    expect(
      validateSandboxRuntimePolicy({
        ...fullPolicy,
        allowed_executables: ['node --eval'],
      }),
    ).toHaveProperty('allowed_executables');
    expect(
      validateSandboxRuntimePolicy({
        ...fullPolicy,
        node_modules_directory_ref: '/host/node_modules',
      }),
    ).toHaveProperty('node_modules_directory_ref');
    expect(
      validateSandboxRuntimePolicy({
        ...fullPolicy,
        timeout_seconds: 3601,
        memory_limit_mb: 32,
      }),
    ).toMatchObject({
      timeout_seconds: expect.any(String),
      memory_limit_mb: expect.any(String),
    });
    expect(getScopeLabel('mcp_stdio')).toBe('MCP stdio');
    expect(getScopeLabel('plugin')).toBe('代码插件');
    expect(SANDBOX_SCOPE_OPTIONS).toContainEqual({
      value: 'plugin',
      label: '代码插件',
      description: '代码插件的受控执行环境',
    });
    expect(getSandboxScopeOptions('remote_http')).toContainEqual(
      expect.objectContaining({ value: 'plugin' }),
    );
    expect(getSandboxScopeOptions('local_debug')).not.toContainEqual(
      expect.objectContaining({ value: 'plugin' }),
    );
    expect(
      filterSandboxScopesForProviderType('local_debug', ['agent', 'plugin']),
    ).toEqual(['agent']);
  });
});
