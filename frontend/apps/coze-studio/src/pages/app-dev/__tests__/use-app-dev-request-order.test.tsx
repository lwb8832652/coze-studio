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

import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import { act } from 'react-dom/test-utils';
import { createRoot, type Root } from 'react-dom/client';

import {
  getAppDevFileContent,
  getAppDevProject,
  getAppDevRuntimeStatus,
  listAppDevFiles,
  listAppDevModels,
  listAppDevProjects,
  listAppDevRuntimeLogs,
  saveAppDevFileContent,
} from '../service';
import { useAppDevRuntime } from '../hooks/use-app-dev-runtime';
import { useAppDevProjects } from '../hooks/use-app-dev-projects';
import { useAppDevProjectInfo } from '../hooks/use-app-dev-project-info';
import { useAppDevModelSelector } from '../hooks/use-app-dev-model-selector';
import { useAppDevFiles } from '../hooks/use-app-dev-files';

globalThis.IS_REACT_ACT_ENVIRONMENT = true;

vi.mock('../service', () => ({
  archiveAppDevProject: vi.fn(),
  createAppDevProject: vi.fn(),
  deleteAppDevFile: vi.fn(),
  duplicateAppDevProject: vi.fn(),
  getAppDevFileContent: vi.fn(),
  getAppDevProject: vi.fn(),
  getAppDevRuntimeStatus: vi.fn(),
  importAppDevProject: vi.fn(),
  keepAliveAppDevRuntime: vi.fn(),
  listAppDevFiles: vi.fn(),
  listAppDevModels: vi.fn(),
  listAppDevProjects: vi.fn(),
  listAppDevRuntimeLogs: vi.fn(),
  normalizeAppDevError: vi.fn(error => String(error)),
  renameAppDevFile: vi.fn(),
  restartAppDevRuntime: vi.fn(),
  saveAppDevFileContent: vi.fn(),
  startAppDevRuntime: vi.fn(),
  stopAppDevRuntime: vi.fn(),
  updateAppDevProject: vi.fn(),
  uploadAppDevFiles: vi.fn(),
}));

const deferred = <T,>() => {
  let resolve!: (value: T) => void;
  const promise = new Promise<T>(next => {
    resolve = next;
  });
  return { promise, resolve };
};

const flushPromises = async () => {
  await Promise.resolve();
  await Promise.resolve();
};

describe('AppDev hooks request ordering', () => {
  let root: Root | undefined;

  beforeEach(() => {
    vi.useFakeTimers();
    vi.mocked(listAppDevFiles).mockResolvedValue({ items: [] });
    vi.mocked(listAppDevRuntimeLogs).mockResolvedValue({ items: [] });
    vi.mocked(saveAppDevFileContent).mockResolvedValue({
      path: 'src/App.tsx',
      content: '',
      version: '1',
    });
  });

  afterEach(() => {
    if (root) {
      act(() => root?.unmount());
      root = undefined;
    }
    vi.useRealTimers();
    vi.clearAllMocks();
  });

  it('keeps the latest opened file and blocks saving a stale draft', async () => {
    const fileA = deferred<{
      path: string;
      content: string;
      version: string;
    }>();
    const fileB = deferred<{
      path: string;
      content: string;
      version: string;
    }>();
    vi.mocked(getAppDevFileContent).mockImplementation(({ path }) =>
      path === 'src/A.tsx' ? fileA.promise : fileB.promise,
    );

    let hook: ReturnType<typeof useAppDevFiles> | undefined;
    const Harness = () => {
      hook = useAppDevFiles('space-1', 'project-1');
      return null;
    };

    root = createRoot(document.createElement('div'));
    await act(async () => {
      root?.render(<Harness />);
      await flushPromises();
    });

    let openA: Promise<void> | undefined;
    await act(async () => {
      openA = hook?.openFile('src/A.tsx');
      fileA.resolve({
        path: 'src/A.tsx',
        content: 'const file = "A";',
        version: '1',
      });
      await openA;
    });
    act(() => hook?.setDraft('const file = "A edited";'));

    let openB: Promise<void> | undefined;
    await act(async () => {
      openB = hook?.openFile('src/B.tsx');
      await flushPromises();
    });
    await act(async () => {
      await hook?.saveCurrentFile();
    });
    expect(saveAppDevFileContent).not.toHaveBeenCalled();

    await act(async () => {
      fileB.resolve({
        path: 'src/B.tsx',
        content: 'const file = "B";',
        version: '1',
      });
      await openB;
    });
    expect(hook?.selectedPath).toBe('src/B.tsx');
    expect(hook?.draft).toBe('const file = "B";');
  });

  it('ignores a project list response from the previous space', async () => {
    const oldSpace = deferred<{ items: never[]; total: number }>();
    const newSpace = deferred<{ items: never[]; total: number }>();
    vi.mocked(listAppDevProjects).mockImplementation(({ spaceId }) =>
      spaceId === 'space-1' ? oldSpace.promise : newSpace.promise,
    );

    let hook: ReturnType<typeof useAppDevProjects> | undefined;
    const Harness = ({ spaceId }: { spaceId: string }) => {
      hook = useAppDevProjects(spaceId);
      return null;
    };

    root = createRoot(document.createElement('div'));
    await act(async () => {
      root?.render(<Harness spaceId="space-1" />);
      await flushPromises();
    });
    let oldRequest: Promise<void> | undefined;
    await act(async () => {
      oldRequest = hook?.refresh();
      await flushPromises();
    });

    await act(async () => {
      root?.render(<Harness spaceId="space-2" />);
      await flushPromises();
    });
    let newRequest: Promise<void> | undefined;
    await act(async () => {
      newRequest = hook?.refresh();
      await flushPromises();
    });

    await act(async () => {
      newSpace.resolve({
        items: [{ id: 'new-project', name: 'New project' } as never],
        total: 1,
      });
      await newRequest;
    });
    await act(async () => {
      oldSpace.resolve({
        items: [{ id: 'old-project', name: 'Old project' } as never],
        total: 1,
      });
      await oldRequest;
    });

    expect(hook?.projects).toEqual([
      expect.objectContaining({ id: 'new-project' }),
    ]);
  });

  it('ignores project details from the previous project', async () => {
    const oldProject = deferred<never>();
    const newProject = deferred<never>();
    vi.mocked(getAppDevProject).mockImplementation(({ projectId }) =>
      projectId === 'project-1' ? oldProject.promise : newProject.promise,
    );

    let hook: ReturnType<typeof useAppDevProjectInfo> | undefined;
    const Harness = ({ projectId }: { projectId: string }) => {
      hook = useAppDevProjectInfo('space-1', projectId);
      return null;
    };

    root = createRoot(document.createElement('div'));
    await act(async () => {
      root?.render(<Harness projectId="project-1" />);
      await flushPromises();
    });
    await act(async () => {
      root?.render(<Harness projectId="project-2" />);
      await flushPromises();
    });
    await act(async () => {
      newProject.resolve({ id: 'project-2', name: 'New project' } as never);
      await flushPromises();
      oldProject.resolve({ id: 'project-1', name: 'Old project' } as never);
      await flushPromises();
    });

    expect(hook?.project).toEqual(expect.objectContaining({ id: 'project-2' }));
  });

  it('ignores models returned for the previous space', async () => {
    const oldModels = deferred<{ items: never[] }>();
    const newModels = deferred<{ items: never[] }>();
    vi.mocked(listAppDevModels).mockImplementation(({ spaceId }) =>
      spaceId === 'space-1' ? oldModels.promise : newModels.promise,
    );

    let hook: ReturnType<typeof useAppDevModelSelector> | undefined;
    const Harness = ({ spaceId }: { spaceId: string }) => {
      hook = useAppDevModelSelector(spaceId);
      return null;
    };

    root = createRoot(document.createElement('div'));
    await act(async () => {
      root?.render(<Harness spaceId="space-1" />);
      await flushPromises();
    });
    await act(async () => {
      root?.render(<Harness spaceId="space-2" />);
      await flushPromises();
    });
    await act(async () => {
      newModels.resolve({
        items: [{ id: 'model-2', name: 'New model' } as never],
      });
      await flushPromises();
      oldModels.resolve({
        items: [{ id: 'model-1', name: 'Old model' } as never],
      });
      await flushPromises();
    });

    expect(hook?.selectedModelId).toBe('model-2');
  });

  it('ignores runtime status returned for the previous project', async () => {
    const oldRuntime = deferred<never>();
    const newRuntime = deferred<never>();
    vi.mocked(getAppDevRuntimeStatus).mockImplementation(({ projectId }) =>
      projectId === 'project-1' ? oldRuntime.promise : newRuntime.promise,
    );

    let hook: ReturnType<typeof useAppDevRuntime> | undefined;
    const Harness = ({ projectId }: { projectId: string }) => {
      hook = useAppDevRuntime('space-1', projectId);
      return null;
    };

    root = createRoot(document.createElement('div'));
    await act(async () => {
      root?.render(<Harness projectId="project-1" />);
      await flushPromises();
    });
    await act(async () => {
      root?.render(<Harness projectId="project-2" />);
      await flushPromises();
    });
    await act(async () => {
      newRuntime.resolve({
        status: 'running',
        message: 'new runtime',
      } as never);
      await flushPromises();
      oldRuntime.resolve({
        status: 'running',
        message: 'old runtime',
      } as never);
      await flushPromises();
    });

    expect(hook?.runtime.message).toBe('new runtime');
  });
});
