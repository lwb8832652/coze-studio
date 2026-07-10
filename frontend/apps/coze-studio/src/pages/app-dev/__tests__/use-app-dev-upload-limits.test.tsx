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

import { afterEach, describe, expect, it, vi } from 'vitest';
import { act } from 'react-dom/test-utils';
import { createRoot, type Root } from 'react-dom/client';

import { listAppDevFiles, uploadAppDevFiles } from '../service';
import { useAppDevFiles } from '../hooks/use-app-dev-files';

globalThis.IS_REACT_ACT_ENVIRONMENT = true;

vi.mock('../service', () => ({
  deleteAppDevFile: vi.fn(),
  getAppDevFileContent: vi.fn(),
  listAppDevFiles: vi.fn(),
  normalizeAppDevError: vi.fn(error => String(error)),
  renameAppDevFile: vi.fn(),
  saveAppDevFileContent: vi.fn(),
  uploadAppDevFiles: vi.fn(),
}));

describe('useAppDevFiles upload limits', () => {
  let root: Root | undefined;

  afterEach(() => {
    if (root) {
      act(() => root?.unmount());
      root = undefined;
    }
    vi.clearAllMocks();
  });

  it('rejects more than one hundred files before sending a request', async () => {
    vi.mocked(listAppDevFiles).mockResolvedValue({ items: [] });
    let hook: ReturnType<typeof useAppDevFiles> | undefined;
    const Harness = () => {
      hook = useAppDevFiles('space-1', 'project-1');
      return null;
    };

    root = createRoot(document.createElement('div'));
    await act(async () => {
      root?.render(<Harness />);
      await Promise.resolve();
      await Promise.resolve();
    });

    const files = Array.from(
      { length: 101 },
      (_, index) => new File(['x'], `file-${index}.txt`),
    );
    await act(async () => {
      await hook?.uploadFiles(files);
    });

    expect(uploadAppDevFiles).not.toHaveBeenCalled();
    expect(hook?.error).toContain('不能超过 100 个文件');
  });
});
