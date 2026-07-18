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

import { StrictMode } from 'react';
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import { act } from 'react-dom/test-utils';
import { createRoot, type Root } from 'react-dom/client';

import { buildAppDevProject, getAppDevBuildStatus } from '../service';
import { useAppDevBuild } from '../hooks/use-app-dev-build';
import {
  readAppDevPendingOperation,
  writeAppDevPendingOperation,
} from '../utils/provider-operation';

globalThis.IS_REACT_ACT_ENVIRONMENT = true;

vi.mock('../service', () => ({
  AppDevSafeError: class AppDevSafeError extends Error {},
  buildAppDevProject: vi.fn(),
  getAppDevBuildStatus: vi.fn(),
  isAppDevDeterministicError: vi.fn(() => false),
  normalizeAppDevError: vi.fn(() => '构建状态确认超时，请点击刷新继续确认'),
}));

const flush = async () => {
  await Promise.resolve();
  await Promise.resolve();
  await Promise.resolve();
};

describe('useAppDevBuild safety boundaries', () => {
  const scope = {
    spaceId: 'space-1',
    projectId: 'project-1',
    principalId: 'user-1',
  };
  let root: Root | undefined;
  let hook: ReturnType<typeof useAppDevBuild> | undefined;

  const render = async (strict = false) => {
    const Harness = () => {
      hook = useAppDevBuild(scope.spaceId, scope.projectId, scope.principalId);
      return null;
    };
    root = createRoot(document.createElement('div'));
    await act(async () => {
      root?.render(
        strict ? (
          <StrictMode>
            <Harness />
          </StrictMode>
        ) : (
          <Harness />
        ),
      );
      await flush();
    });
  };

  beforeEach(() => {
    vi.useFakeTimers();
    sessionStorage.clear();
  });

  afterEach(async () => {
    if (root) {
      await act(async () => {
        root?.unmount();
        await flush();
      });
    }
    root = undefined;
    hook = undefined;
    vi.clearAllTimers();
    vi.useRealTimers();
    vi.restoreAllMocks();
    sessionStorage.clear();
  });

  it('times out a never-settling POST and reconciles the same operation', async () => {
    let firstSignal: AbortSignal | undefined;
    vi.mocked(buildAppDevProject).mockImplementationOnce(input => {
      firstSignal = input.signal;
      return new Promise(() => undefined);
    });
    await render();

    let first: Promise<unknown> | undefined;
    act(() => {
      first = hook?.beginBuild();
    });
    await act(async () => {
      await vi.advanceTimersByTimeAsync(15_001);
      await first;
    });

    const pending = readAppDevPendingOperation(scope, 'build');
    expect(firstSignal?.aborted).toBe(true);
    expect(hook?.building).toBe(false);
    expect(pending).toBeDefined();

    vi.mocked(buildAppDevProject).mockResolvedValueOnce({
      generation: 21,
      state: 'ready',
      releaseAvailable: true,
      size: 8,
      stale: false,
    });
    await act(async () => {
      await hook?.reconcile();
    });
    expect(vi.mocked(buildAppDevProject).mock.calls[1][0].operationId).toBe(
      pending?.operationId,
    );
    expect(hook?.projection.state).toBe('ready');
  });

  it('bounds a never-settling GET and preserves its pending reconciliation', async () => {
    vi.mocked(buildAppDevProject).mockResolvedValue({
      generation: 22,
      state: 'building',
      releaseAvailable: false,
      size: 0,
      stale: false,
    });
    let pollSignal: AbortSignal | undefined;
    vi.mocked(getAppDevBuildStatus).mockImplementation(input => {
      pollSignal = input.signal;
      return new Promise(() => undefined);
    });
    await render();
    await act(async () => hook?.beginBuild());

    await act(async () => {
      await vi.advanceTimersByTimeAsync(15_501);
      await flush();
    });

    expect(pollSignal?.aborted).toBe(true);
    expect(readAppDevPendingOperation(scope, 'build')).toMatchObject({
      generation: 22,
    });
  });

  it('uses one logical operation during StrictMode effect replay', async () => {
    writeAppDevPendingOperation(scope, 'build', {
      operationId: 'strict-build-operation',
      phase: 'requesting',
    });
    const signals: AbortSignal[] = [];
    vi.mocked(buildAppDevProject).mockImplementation(input => {
      signals.push(input.signal!);
      return new Promise(() => undefined);
    });

    await render(true);

    expect(
      new Set(
        vi
          .mocked(buildAppDevProject)
          .mock.calls.map(call => call[0].operationId),
      ),
    ).toEqual(new Set(['strict-build-operation']));
    expect(signals[0]?.aborted).toBe(true);
  });
});
