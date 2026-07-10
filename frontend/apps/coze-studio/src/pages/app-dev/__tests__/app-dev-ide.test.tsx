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

/* eslint-disable @typescript-eslint/require-await -- Async mocks mirror production contracts. */

import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import { act } from 'react-dom/test-utils';
import { createRoot, type Root } from 'react-dom/client';

import {
  getAppDevFileContent,
  listAppDevFiles,
  saveAppDevFileContent,
} from '../service';
import { useAppDevFiles } from '../hooks/use-app-dev-files';

globalThis.IS_REACT_ACT_ENVIRONMENT = true;

vi.mock('../service', () => ({
  deleteAppDevFile: vi.fn(),
  getAppDevFileContent: vi.fn(),
  listAppDevFiles: vi.fn(),
  normalizeAppDevError: vi.fn(error =>
    error instanceof Error ? error.message : String(error),
  ),
  renameAppDevFile: vi.fn(),
  saveAppDevFileContent: vi.fn(),
  uploadAppDevFiles: vi.fn(),
}));

describe('useAppDevFiles', () => {
  let root: Root | undefined;
  let files: ReturnType<typeof useAppDevFiles> | undefined;

  const Harness = () => {
    files = useAppDevFiles('space-1', 'project-1');
    return null;
  };

  beforeEach(() => {
    vi.mocked(listAppDevFiles).mockResolvedValue({
      items: [
        {
          id: 'src/App.tsx',
          path: 'src/App.tsx',
          name: 'App.tsx',
          type: 'file',
        },
      ],
      total: 1,
    });
    vi.mocked(getAppDevFileContent).mockResolvedValue({
      path: 'src/App.tsx',
      content: 'export default function App() { return null; }',
      version: 'v1',
      language: 'typescript',
    });
    vi.mocked(saveAppDevFileContent).mockImplementation(async params => ({
      path: params.path,
      content: params.content,
      version: 'v2',
      language: 'typescript',
    }));
  });

  afterEach(() => {
    if (root) {
      act(() => root?.unmount());
      root = undefined;
    }
    files = undefined;
    vi.clearAllMocks();
  });

  const renderHook = async () => {
    root = createRoot(document.createElement('div'));
    await act(async () => {
      root?.render(<Harness />);
      await Promise.resolve();
      await Promise.resolve();
    });
  };

  it('loads, edits, and saves a selected file with optimistic version control', async () => {
    await renderHook();
    expect(files?.tree).toHaveLength(1);

    await act(async () => {
      await files?.openFile('src/App.tsx');
    });
    expect(files?.selectedPath).toBe('src/App.tsx');
    expect(files?.dirty).toBe(false);

    act(() =>
      files?.setDraft('export default function App() { return <main />; }'),
    );
    expect(files?.dirty).toBe(true);

    await act(async () => {
      await files?.saveCurrentFile();
    });

    expect(saveAppDevFileContent).toHaveBeenCalledWith({
      spaceId: 'space-1',
      projectId: 'project-1',
      path: 'src/App.tsx',
      content: 'export default function App() { return <main />; }',
      version: 'v1',
    });
    expect(files?.fileContent?.version).toBe('v2');
    expect(files?.dirty).toBe(false);
  });

  it('blocks save when no file is selected', async () => {
    await renderHook();

    await act(async () => {
      await files?.saveCurrentFile();
    });

    expect(saveAppDevFileContent).not.toHaveBeenCalled();
    expect(files?.error).toBe('请先选择要保存的文件');
  });
});
