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

/* eslint-disable @typescript-eslint/require-await, @typescript-eslint/consistent-type-imports, @typescript-eslint/naming-convention -- Test doubles preserve async and component-shaped import contracts. */

import { StrictMode } from 'react';

import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import { act } from 'react-dom/test-utils';
import { createRoot, type Root } from 'react-dom/client';

import { writeAppDevPendingOperation } from '../utils/provider-operation';
import {
  AppDevSafeError,
  downloadAppDevRelease,
  exportAppDevProject,
  listAppDevSnapshots,
  restoreAppDevSnapshot,
} from '../service';
import AppDevIDEPage from '../ide';

globalThis.IS_REACT_ACT_ENVIRONMENT = true;

const mocks = vi.hoisted(() => ({
  params: { space_id: 'space-1', project_id: 'project-1' },
  navigate: vi.fn(),
  refreshTree: vi.fn(async () => undefined),
  openFile: vi.fn(async () => undefined),
  markStale: vi.fn(),
  reconcileBuild: vi.fn(async () => undefined),
  runtimeStatus: 'running' as
    | 'starting'
    | 'running'
    | 'recovering'
    | 'stopping'
    | 'cleanup_pending'
    | 'stopped'
    | 'error',
  runtimeLoading: false,
  previewUrl: 'https://preview.example.test/app',
  refreshRuntime: vi.fn(async () => undefined),
  refreshLogs: vi.fn(async () => undefined),
  principalId: 'user-1',
  confirm: vi.fn(async () => true),
}));

vi.mock('@coze-arch/foundation-sdk', () => ({
  useUserInfo: () => ({ user_id_str: mocks.principalId }),
}));

vi.mock('react-router-dom', () => ({
  useNavigate: () => mocks.navigate,
  useParams: () => mocks.params,
}));

vi.mock('../service', async importOriginal => ({
  ...(await importOriginal<typeof import('../service')>()),
  createAppDevSnapshot: vi.fn(),
  downloadAppDevRelease: vi.fn(),
  exportAppDevProject: vi.fn(),
  importAppDevProject: vi.fn(),
  listAppDevSnapshots: vi.fn(),
  restoreAppDevSnapshot: vi.fn(),
  updateAppDevProject: vi.fn(),
}));

vi.mock('../hooks/use-app-dev-project-info', () => ({
  useAppDevProjectInfo: (_spaceId: string, projectId: string) => ({
    project: {
      id: projectId,
      name: `Project ${projectId}`,
      description: 'provider project',
    },
    loading: false,
    error: '',
    refresh: vi.fn(async () => undefined),
  }),
}));

vi.mock('../hooks/use-app-dev-files', () => ({
  useAppDevFiles: () => ({
    tree: [],
    selectedPath: 'src/App.tsx',
    dirty: false,
    draft: 'export default function App() {}',
    loadingTree: false,
    loadingContent: false,
    saving: false,
    error: '',
    refreshTree: mocks.refreshTree,
    openFile: mocks.openFile,
    saveCurrentFile: vi.fn(async () => true),
    setDraft: vi.fn(),
    createFile: vi.fn(),
    createDirectory: vi.fn(),
    uploadFiles: vi.fn(),
    renamePath: vi.fn(),
    deletePath: vi.fn(),
  }),
}));

vi.mock('../hooks/use-app-dev-runtime', () => ({
  useAppDevRuntime: () => ({
    runtime: {
      generation: 2,
      status: mocks.runtimeStatus,
      canStart:
        mocks.runtimeStatus === 'stopped' || mocks.runtimeStatus === 'error',
      recovering: mocks.runtimeStatus === 'recovering',
      stopping: ['stopping', 'cleanup_pending'].includes(mocks.runtimeStatus),
      previewUrl: mocks.previewUrl,
    },
    logs: [],
    loading: mocks.runtimeLoading,
    logsLoading: false,
    error: '',
    start: vi.fn(),
    stop: vi.fn(),
    restart: vi.fn(),
    refreshStatus: mocks.refreshRuntime,
    refreshLogs: mocks.refreshLogs,
  }),
}));

vi.mock('../hooks/use-app-dev-build', () => ({
  useAppDevBuild: () => ({
    projection: {
      generation: 2,
      state: 'ready',
      releaseAvailable: true,
      size: 42,
      stale: false,
    },
    building: false,
    error: '',
    beginBuild: vi.fn(),
    reconcile: mocks.reconcileBuild,
    markStale: mocks.markStale,
  }),
}));

vi.mock('../components/chat-panel', () => ({
  APP_DEV_DESIGN_TARGETS: ['全局视觉系统', '首页'],
  ChatPanel: () => <div>AI 编辑助手</div>,
}));
vi.mock('../components/file-tree', () => ({
  FileTree: () => <div>项目文件树</div>,
}));
vi.mock('../components/code-editor', () => ({
  CodeEditor: () => <div>文件编辑器</div>,
}));
vi.mock('../components/dev-logs-panel', () => ({
  DevLogsPanel: () => <div>运行日志内容</div>,
}));
vi.mock('../components/import-project-modal', () => ({
  ImportProjectModal: () => null,
}));
vi.mock('../components/text-input-dialog', () => ({
  useConfirmDialog: () => ({
    openConfirmDialog: mocks.confirm,
    confirmDialog: null,
  }),
}));

const deferred = <T,>() => {
  let resolve!: (value: T) => void;
  const promise = new Promise<T>(next => (resolve = next));
  return { promise, resolve };
};

const flush = async () => {
  await Promise.resolve();
  await Promise.resolve();
  await Promise.resolve();
};

describe('AppDev IDE provider integration', () => {
  let root: Root | undefined;
  let container: HTMLDivElement;

  beforeEach(() => {
    container = document.createElement('div');
    document.body.append(container);
    root = createRoot(container);
    mocks.params = { space_id: 'space-1', project_id: 'project-1' };
    mocks.runtimeStatus = 'running';
    mocks.runtimeLoading = false;
    mocks.previewUrl = 'https://preview.example.test/app';
    mocks.principalId = 'user-1';
    mocks.confirm.mockResolvedValue(true);
    sessionStorage.clear();
    vi.mocked(listAppDevSnapshots).mockResolvedValue({ items: [] });
  });

  afterEach(async () => {
    if (root) {
      await act(async () => {
        root?.unmount();
        await flush();
      });
    }
    root = undefined;
    container.remove();
    document.body.style.overflow = '';
    vi.clearAllMocks();
    vi.restoreAllMocks();
    sessionStorage.clear();
  });

  const render = async (strict = false) => {
    await act(async () => {
      root?.render(
        strict ? (
          <StrictMode>
            <AppDevIDEPage />
          </StrictMode>
        ) : (
          <AppDevIDEPage />
        ),
      );
      await flush();
    });
  };

  it('keeps snapshot, logs, device, fullscreen, design, and file editing entry points', async () => {
    await render();
    for (const text of [
      '保存快照',
      '版本历史',
      '开发日志',
      '桌面',
      '平板',
      '手机',
      '全屏',
      '设计模式',
      '代码',
    ]) {
      expect(container.textContent).toContain(text);
    }
    const codeTab = Array.from(container.querySelectorAll('button')).find(
      button => button.textContent?.includes('代码'),
    );
    await act(async () => codeTab?.click());
    expect(container.textContent).toContain('文件编辑器');
  });

  it.each([
    ['starting', '启动中', true],
    ['running', '运行中', false],
    ['recovering', '恢复中', true],
    ['stopping', '停止中', true],
    ['cleanup_pending', '清理中', true],
    ['stopped', '未启动', true],
    ['error', '异常', true],
  ] as const)(
    'links real IDE controls and status copy in %s',
    async (status, statusCopy, disabled) => {
      mocks.runtimeStatus = status;
      await render();
      const toolbarBuild = container.querySelector<HTMLButtonElement>(
        '.app-dev-runtime-toolbar__publish-action',
      );
      const overviewBuild = container.querySelector<HTMLButtonElement>(
        '.app-dev-release-panel__actions button',
      );
      const fullscreen = Array.from(container.querySelectorAll('button')).find(
        button => button.textContent === '全屏预览',
      );
      expect(toolbarBuild?.disabled).toBe(disabled);
      expect(overviewBuild?.disabled).toBe(disabled);
      expect(fullscreen?.disabled).toBe(disabled);
      expect(container.textContent).toContain(statusCopy);
    },
  );

  it('disables fullscreen and keeps a safe empty state for an unsafe running URL', async () => {
    mocks.runtimeStatus = 'running';
    mocks.previewUrl = 'javascript:alert(1)';
    await render();
    const fullscreen = Array.from(container.querySelectorAll('button')).find(
      button => button.textContent === '全屏预览',
    );
    expect(fullscreen?.disabled).toBe(true);
    fullscreen?.click();
    expect(container.querySelector('[role="dialog"]')).toBeNull();
    expect(container.querySelector('iframe')).toBeNull();
    expect(container.textContent).toContain('预览地址尚未就绪');
  });

  it('opens fullscreen from the trusted running preview capability', async () => {
    mocks.runtimeStatus = 'running';
    mocks.previewUrl = 'https://preview.example.test/app';
    await render();
    const fullscreen = Array.from(container.querySelectorAll('button')).find(
      button => button.textContent === '全屏预览',
    );
    expect(fullscreen?.disabled).toBe(false);
    await act(async () => fullscreen?.click());
    expect(container.querySelector('[role="dialog"]')).not.toBeNull();
    expect(
      Array.from(container.querySelectorAll('iframe')).every(
        iframe =>
          iframe.getAttribute('src') === 'https://preview.example.test/app',
      ),
    ).toBe(true);
  });

  it('provides accessible project settings and nested snapshot dialogs', async () => {
    await render();
    const settings = container.querySelector<HTMLButtonElement>(
      '[aria-label="编辑项目设置"]',
    )!;
    settings.focus();
    await act(async () => {
      settings.click();
      await flush();
    });
    let dialog = container.querySelector<HTMLElement>(
      '[role="dialog"][aria-label="项目设置"]',
    )!;
    expect(dialog.getAttribute('aria-modal')).toBe('true');
    expect(document.activeElement).toBe(
      dialog.querySelector('input[data-dialog-autofocus]'),
    );
    await act(async () => {
      window.dispatchEvent(new KeyboardEvent('keydown', { key: 'Escape' }));
      await flush();
    });
    expect(container.querySelector('[aria-label="项目设置"]')).toBeNull();
    expect(document.activeElement).toBe(settings);

    const history = Array.from(container.querySelectorAll('button')).find(
      button => button.textContent === '版本历史',
    )!;
    history.focus();
    await act(async () => {
      history.click();
      await flush();
    });
    dialog = container.querySelector<HTMLElement>(
      '[role="dialog"][aria-label="版本历史"]',
    )!;
    expect(dialog.getAttribute('aria-modal')).toBe('true');
    const saveSnapshot = Array.from(dialog.querySelectorAll('button')).find(
      button => button.textContent === '保存新快照',
    )!;
    saveSnapshot.focus();
    await act(async () => {
      saveSnapshot.click();
      await flush();
    });
    const nested = container.querySelector<HTMLElement>(
      '[role="dialog"][aria-label="保存快照"]',
    )!;
    expect(document.activeElement).toBe(
      nested.querySelector('input[data-dialog-autofocus]'),
    );
    await act(async () => {
      window.dispatchEvent(new KeyboardEvent('keydown', { key: 'Escape' }));
      await flush();
    });
    expect(container.querySelector('[aria-label="保存快照"]')).toBeNull();
    expect(document.activeElement).toBe(saveSnapshot);
    await act(async () => {
      window.dispatchEvent(new KeyboardEvent('keydown', { key: 'Escape' }));
      await flush();
    });
    expect(container.querySelector('[aria-label="版本历史"]')).toBeNull();
    expect(document.activeElement).toBe(history);
  });

  it('aborts and fences a stale snapshot list response after identity changes', async () => {
    const oldList = deferred<Awaited<ReturnType<typeof listAppDevSnapshots>>>();
    let oldSignal: AbortSignal | undefined;
    vi.mocked(listAppDevSnapshots).mockImplementationOnce(input => {
      oldSignal = input.signal;
      return oldList.promise;
    });
    await render();
    const history = Array.from(container.querySelectorAll('button')).find(
      button => button.textContent === '版本历史',
    );
    await act(async () => {
      history?.click();
      await flush();
    });
    expect(oldSignal?.aborted).toBe(false);

    mocks.params = { space_id: 'space-1', project_id: 'project-2' };
    await render();
    expect(oldSignal?.aborted).toBe(true);
    await act(async () => {
      oldList.resolve({
        items: [
          {
            id: 'old-project-snapshot',
            label: '旧项目快照',
            createdAt: 'now',
          },
        ],
      });
      await flush();
    });
    expect(container.textContent).not.toContain('旧项目快照');
    expect(container.textContent).not.toContain('加载快照中');
  });

  it('aborts and ignores a snapshot restore response from the previous project', async () => {
    const restore =
      deferred<Awaited<ReturnType<typeof restoreAppDevSnapshot>>>();
    vi.mocked(listAppDevSnapshots).mockResolvedValue({
      items: [{ id: 'snapshot-1', label: 'Snapshot One', createdAt: 'now' }],
    });
    vi.mocked(restoreAppDevSnapshot).mockImplementation(() => restore.promise);
    await render();
    const history = Array.from(container.querySelectorAll('button')).find(
      button => button.textContent === '版本历史',
    );
    await act(async () => {
      history?.click();
      await flush();
    });
    const restoreButton = Array.from(container.querySelectorAll('button')).find(
      button => button.textContent === '恢复',
    );
    await act(async () => {
      restoreButton?.click();
      await flush();
    });
    expect(restoreAppDevSnapshot).toHaveBeenCalledWith(
      expect.objectContaining({
        projectId: 'project-1',
        snapshotId: 'snapshot-1',
        operationId: expect.any(String),
        signal: expect.any(AbortSignal),
      }),
    );

    mocks.params = { space_id: 'space-1', project_id: 'project-2' };
    await render();
    await act(async () => {
      restore.resolve({
        generation: 3,
        status: 'running',
        canStart: false,
        recovering: false,
        stopping: false,
      });
      await flush();
    });
    expect(mocks.refreshTree).not.toHaveBeenCalled();
    expect(container.textContent).not.toContain('已恢复到快照');
  });

  it('marks stale release without creating an object URL', async () => {
    vi.mocked(downloadAppDevRelease).mockRejectedValue(
      new AppDevSafeError('release_stale'),
    );
    const createObjectURL = vi
      .spyOn(URL, 'createObjectURL')
      .mockReturnValue('blob:unexpected');
    await render();
    const download = Array.from(container.querySelectorAll('button')).find(
      button => button.textContent === '下载产物' && !button.disabled,
    );
    await act(async () => {
      download?.click();
      await flush();
    });
    expect(mocks.markStale).toHaveBeenCalledTimes(1);
    expect(mocks.reconcileBuild).toHaveBeenCalledTimes(1);
    expect(createObjectURL).not.toHaveBeenCalled();
  });

  it('recovers a persisted snapshot intent with the same key after reload', async () => {
    writeAppDevPendingOperation(
      {
        spaceId: 'space-1',
        projectId: 'project-1',
        principalId: 'user-1',
      },
      'snapshot-restore',
      {
        operationId: 'persisted-snapshot-operation',
        snapshotId: 'snapshot-persisted',
        phase: 'requesting',
      },
    );
    vi.mocked(restoreAppDevSnapshot).mockResolvedValue({
      generation: 3,
      status: 'running',
      canStart: false,
      recovering: false,
      stopping: false,
    });
    await render();
    expect(restoreAppDevSnapshot).toHaveBeenCalledWith(
      expect.objectContaining({
        snapshotId: 'snapshot-persisted',
        operationId: 'persisted-snapshot-operation',
      }),
    );
  });

  it('does not download a release returned after switching projects', async () => {
    const release = deferred<Blob>();
    vi.mocked(downloadAppDevRelease).mockImplementation(() => release.promise);
    const createObjectURL = vi
      .spyOn(URL, 'createObjectURL')
      .mockReturnValue('blob:unexpected');
    await render();
    const download = Array.from(container.querySelectorAll('button')).find(
      button => button.textContent === '下载产物' && !button.disabled,
    );
    await act(async () => {
      download?.click();
      await flush();
    });
    mocks.params = { space_id: 'space-1', project_id: 'project-2' };
    await render();
    await act(async () => {
      release.resolve(new Blob(['old-project']));
      await flush();
    });
    expect(createObjectURL).not.toHaveBeenCalled();
  });

  it('does not restore after identity changes while confirmation is pending', async () => {
    const confirmation = deferred<boolean>();
    mocks.confirm.mockImplementationOnce(() => confirmation.promise);
    vi.mocked(listAppDevSnapshots).mockResolvedValue({
      items: [{ id: 'snapshot-1', label: 'Snapshot One', createdAt: 'now' }],
    });
    await render();
    const history = Array.from(container.querySelectorAll('button')).find(
      button => button.textContent === '版本历史',
    );
    await act(async () => {
      history?.click();
      await flush();
    });
    const restoreButton = Array.from(container.querySelectorAll('button')).find(
      button => button.textContent === '恢复',
    );
    act(() => restoreButton?.click());
    await act(flush);

    mocks.params = { space_id: 'space-1', project_id: 'project-2' };
    await render();
    await act(async () => {
      confirmation.resolve(true);
      await flush();
    });

    expect(restoreAppDevSnapshot).not.toHaveBeenCalled();
    expect(container.textContent).not.toContain('已恢复到快照');
  });

  it('aborts an export from the previous project and never downloads it', async () => {
    const exported = deferred<Blob>();
    vi.mocked(exportAppDevProject).mockImplementation(() => exported.promise);
    const createObjectURL = vi
      .spyOn(URL, 'createObjectURL')
      .mockReturnValue('blob:old-project');
    await render();
    const exportButton = Array.from(container.querySelectorAll('button')).find(
      button => button.textContent === '导出源码',
    );
    act(() => exportButton?.click());
    await act(flush);
    const signal = vi.mocked(exportAppDevProject).mock.calls[0]?.[0].signal;

    mocks.params = { space_id: 'space-1', project_id: 'project-2' };
    await render();
    expect(signal?.aborted).toBe(true);
    await act(async () => {
      exported.resolve(new Blob(['old project']));
      await flush();
    });

    expect(createObjectURL).not.toHaveBeenCalled();
    expect(container.textContent).not.toContain('项目导出已开始下载');
  });

  it('replays a persisted snapshot safely under StrictMode', async () => {
    writeAppDevPendingOperation(
      {
        spaceId: 'space-1',
        projectId: 'project-1',
        principalId: 'user-1',
      },
      'snapshot-restore',
      {
        operationId: 'strict-snapshot-operation',
        snapshotId: 'strict-snapshot',
      },
    );
    const signals: AbortSignal[] = [];
    vi.mocked(restoreAppDevSnapshot).mockImplementation(input => {
      signals.push(input.signal!);
      return new Promise(() => undefined);
    });

    await render(true);

    expect(
      new Set(
        vi
          .mocked(restoreAppDevSnapshot)
          .mock.calls.map(call => call[0].operationId),
      ),
    ).toEqual(new Set(['strict-snapshot-operation']));
    expect(signals[0]?.aborted).toBe(true);
  });
});
