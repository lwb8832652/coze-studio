/*
 * Copyright 2025 coze-dev Authors
 *
 * Licensed under the Apache License, Version 2.0 (the "License");
 * you may not use this file except in compliance with the License.
 * You may obtain a copy of the License at
 *
 *     http://www.apache.org/licenses/LICENSE-2.0
 *
 * Unless required by applicable law or agreed to in writing, software
 * distributed under the License is distributed on an "AS IS" BASIS,
 * WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
 * See the License for the specific language governing permissions and
 * limitations under the License.
 */

/* eslint-disable @typescript-eslint/consistent-type-imports -- Dynamic import types describe the mocked module contract. */

import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import { act } from 'react-dom/test-utils';
import { createRoot, type Root } from 'react-dom/client';

import {
  readAppDevPendingOperation,
  readAppDevTerminalBuild,
  writeAppDevPendingOperation,
  writeAppDevTerminalBuild,
} from '../utils/provider-operation';
import {
  buildAppDevProject,
  getAppDevBuildStatus,
  isAppDevDeterministicError,
} from '../service';
import { useAppDevBuild } from '../hooks/use-app-dev-build';

globalThis.IS_REACT_ACT_ENVIRONMENT = true;

vi.mock('../service', async importOriginal => {
  const actual = await importOriginal<typeof import('../service')>();
  return {
    ...actual,
    buildAppDevProject: vi.fn(),
    getAppDevBuildStatus: vi.fn(),
    isAppDevDeterministicError: vi.fn(() => false),
    normalizeAppDevError: vi.fn(() => '安全构建错误'),
  };
});

const flushPromises = async () => {
  await Promise.resolve();
  await Promise.resolve();
};

describe('useAppDevBuild', () => {
  let root: Root | undefined;
  let build: ReturnType<typeof useAppDevBuild> | undefined;
  const operationScope = {
    spaceId: 'space-1',
    projectId: 'project-1',
    principalId: 'user-1',
  };

  const render = async (spaceId = 'space-1', projectId = 'project-1') => {
    const Harness = () => {
      build = useAppDevBuild(spaceId, projectId, 'user-1');
      return null;
    };
    root = createRoot(document.createElement('div'));
    await act(async () => {
      root?.render(<Harness />);
      await flushPromises();
    });
  };

  beforeEach(() => {
    vi.useFakeTimers();
    sessionStorage.clear();
    vi.mocked(isAppDevDeterministicError).mockReturnValue(false);
  });

  afterEach(async () => {
    if (root) {
      await act(async () => {
        root?.unmount();
        await flushPromises();
      });
      root = undefined;
    }
    build = undefined;
    vi.clearAllTimers();
    vi.useRealTimers();
    vi.clearAllMocks();
    sessionStorage.clear();
  });

  it('polls with the same operation and generation then clears terminal state', async () => {
    vi.mocked(buildAppDevProject).mockResolvedValue({
      generation: 9,
      state: 'building',
      releaseAvailable: false,
      size: 0,
      stale: false,
    });
    vi.mocked(getAppDevBuildStatus).mockResolvedValue({
      generation: 9,
      state: 'ready',
      releaseAvailable: true,
      size: 42,
      stale: false,
      updatedAt: '2026-07-17T10:00:00Z',
    });
    await render();
    await act(async () => {
      await build?.beginBuild();
    });
    const { operationId } = vi.mocked(buildAppDevProject).mock.calls[0][0];

    await act(async () => {
      await vi.advanceTimersByTimeAsync(500);
      await flushPromises();
    });
    expect(getAppDevBuildStatus).toHaveBeenCalledWith(
      expect.objectContaining({
        operationId,
        expectedGeneration: 9,
      }),
    );
    expect(build?.projection.state).toBe('ready');
    expect(build?.projection.releaseAvailable).toBe(true);
    expect(readAppDevPendingOperation(operationScope, 'build')).toBeUndefined();
    expect(readAppDevTerminalBuild(operationScope)).toMatchObject({
      operationId,
      projection: { state: 'ready' },
    });
  });

  it('restores and reconciles a terminal ready build after reload', async () => {
    writeAppDevTerminalBuild(operationScope, {
      operationId: 'terminal-build-operation',
      generation: 12,
      projection: {
        generation: 12,
        state: 'ready',
        releaseAvailable: true,
        size: 99,
        stale: false,
      },
    });
    vi.mocked(getAppDevBuildStatus).mockResolvedValue({
      generation: 12,
      state: 'ready',
      releaseAvailable: true,
      size: 99,
      stale: false,
    });
    await render();
    await act(flushPromises);
    expect(build?.projection).toMatchObject({ state: 'ready', size: 99 });
    expect(getAppDevBuildStatus).toHaveBeenCalledWith(
      expect.objectContaining({
        operationId: 'terminal-build-operation',
        expectedGeneration: 12,
      }),
    );
  });

  it('retains a timed-out pending build and exposes explicit reconciliation', async () => {
    vi.mocked(buildAppDevProject).mockResolvedValue({
      generation: 13,
      state: 'building',
      releaseAvailable: false,
      size: 0,
      stale: false,
    });
    vi.mocked(getAppDevBuildStatus).mockResolvedValue({
      generation: 13,
      state: 'building',
      releaseAvailable: false,
      size: 0,
      stale: false,
    });
    await render();
    await act(async () => build?.beginBuild());
    await act(async () => vi.runAllTimersAsync());
    expect(build?.error).toContain('确认超时');
    expect(readAppDevPendingOperation(operationScope, 'build')).toBeDefined();
    expect(typeof build?.reconcile).toBe('function');
  });

  it('clears a terminal reconciliation journal on typed conflict', async () => {
    writeAppDevTerminalBuild(operationScope, {
      operationId: 'terminal-conflict-operation',
      generation: 14,
      projection: {
        generation: 14,
        state: 'ready',
        releaseAvailable: true,
        size: 9,
        stale: false,
      },
    });
    vi.mocked(isAppDevDeterministicError).mockReturnValue(true);
    vi.mocked(getAppDevBuildStatus).mockRejectedValue(
      new Error('raw conflict'),
    );
    await render();
    await act(flushPromises);
    expect(readAppDevTerminalBuild(operationScope)).toBeUndefined();
    expect(build?.projection).toMatchObject({ state: 'idle', stale: true });
  });

  it('recovers a pending build from session storage without RecoverBuild', async () => {
    writeAppDevPendingOperation(operationScope, 'build', {
      operationId: 'persisted-build-operation',
      generation: 4,
      phase: 'observed',
    });
    vi.mocked(getAppDevBuildStatus).mockResolvedValue({
      generation: 4,
      state: 'failed',
      releaseAvailable: false,
      size: 0,
      stale: false,
      safeErrorCode: 'build_failed',
      safeMessage: '构建失败，请检查源码后重试',
    });
    await render();
    await act(async () => {
      await flushPromises();
    });

    expect(getAppDevBuildStatus).toHaveBeenCalledWith(
      expect.objectContaining({
        operationId: 'persisted-build-operation',
        expectedGeneration: 4,
      }),
    );
    expect(buildAppDevProject).not.toHaveBeenCalled();
    expect(build?.projection.state).toBe('failed');
  });

  it('deduplicates repeated build clicks and ignores a stale project response', async () => {
    let resolveBuild!: (
      value: Awaited<ReturnType<typeof buildAppDevProject>>,
    ) => void;
    vi.mocked(buildAppDevProject).mockImplementation(
      () => new Promise(resolve => (resolveBuild = resolve)),
    );
    let projectId = 'project-1';
    const Harness = ({ currentProject }: { currentProject: string }) => {
      build = useAppDevBuild('space-1', currentProject, 'user-1');
      return null;
    };
    root = createRoot(document.createElement('div'));
    await act(async () => {
      root?.render(<Harness currentProject={projectId} />);
      await flushPromises();
    });

    act(() => {
      void build?.beginBuild();
      void build?.beginBuild();
    });
    expect(buildAppDevProject).toHaveBeenCalledTimes(1);

    projectId = 'project-2';
    await act(async () => {
      root?.render(<Harness currentProject={projectId} />);
      await flushPromises();
    });
    await act(async () => {
      resolveBuild({
        generation: 5,
        state: 'ready',
        releaseAvailable: true,
        size: 12,
        stale: false,
      });
      await flushPromises();
    });
    expect(build?.projection.state).toBe('idle');
  });
});
