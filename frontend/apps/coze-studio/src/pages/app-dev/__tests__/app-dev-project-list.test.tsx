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

import type { AppDevProject } from '../types';
import { ProjectList } from '../components/project-list';

globalThis.IS_REACT_ACT_ENVIRONMENT = true;

const projects: AppDevProject[] = [
  {
    id: 'published-project',
    name: '已发布页面',
    description: '生产页面',
    status: 'ready',
    lastBuildStatus: 'success',
    lastBuildType: 'PAGE',
    lastBuildAt: '2026-07-10T04:00:00Z',
    sourceUpdatedAt: '2026-07-10T03:00:00Z',
  },
  {
    id: 'draft-project',
    name: '未发布页面',
    prompt: '创建草稿页面',
    status: 'ready',
  },
];

describe('ProjectList', () => {
  let container: HTMLDivElement;
  let root: Root;

  beforeEach(() => {
    container = document.createElement('div');
    root = createRoot(container);
  });

  afterEach(() => {
    act(() => root.unmount());
    vi.clearAllMocks();
  });

  const renderProjectList = async (
    overrides: Partial<React.ComponentProps<typeof ProjectList>> = {},
  ) => {
    const props: React.ComponentProps<typeof ProjectList> = {
      projects,
      total: projects.length,
      keyword: '',
      onKeywordChange: vi.fn(),
      onRefresh: vi.fn(),
      onCreate: vi.fn(),
      onImport: vi.fn(),
      onOpen: vi.fn(),
      onArchive: vi.fn(),
      onRename: vi.fn(),
      onDuplicate: vi.fn(),
      onExport: vi.fn(),
      ...overrides,
    };

    await act(async () => root.render(<ProjectList {...props} />));
    return props;
  };

  const clickButton = (name: string) => {
    const button = Array.from(container.querySelectorAll('button')).find(
      item => item.textContent?.trim() === name,
    );
    expect(button).toBeTruthy();
    act(() =>
      button?.dispatchEvent(new MouseEvent('click', { bubbles: true })),
    );
  };

  it('filters projects and exposes primary list actions', async () => {
    const onOpen = vi.fn();
    const onCreate = vi.fn();
    const onKeywordChange = vi.fn();
    await renderProjectList({ onOpen, onCreate, onKeywordChange });

    expect(container.textContent).toContain('2 个项目');
    expect(container.textContent).toContain('已发布页面');
    expect(container.textContent).toContain('未发布页面');

    clickButton('已发布');
    expect(container.textContent).toContain('已发布页面');
    expect(container.textContent).not.toContain('未发布页面');

    clickButton('进入开发');
    expect(onOpen).toHaveBeenCalledWith(projects[0]);

    const search = container.querySelector<HTMLInputElement>(
      'input[placeholder="搜索页面名称"]',
    );
    expect(search).toBeTruthy();
    act(() => {
      if (search) {
        Object.getOwnPropertyDescriptor(
          HTMLInputElement.prototype,
          'value',
        )?.set?.call(search, '首页');
        search.dispatchEvent(new Event('input', { bubbles: true }));
      }
    });
    expect(onKeywordChange).toHaveBeenCalledWith('首页');

    clickButton('+ 网页应用');
    expect(onCreate).toHaveBeenCalledTimes(1);
  });

  it('renders loading, error, and empty states with recovery actions', async () => {
    await renderProjectList({ loading: true });
    expect(container.textContent).toContain('正在加载网页应用...');

    const onRefresh = vi.fn();
    await renderProjectList({
      loading: false,
      error: '项目加载失败',
      onRefresh,
    });
    expect(container.textContent).toContain('项目加载失败');
    clickButton('重试');
    expect(onRefresh).toHaveBeenCalledTimes(1);

    const onCreate = vi.fn();
    await renderProjectList({ projects: [], total: 0, error: '', onCreate });
    expect(container.textContent).toContain('还没有网页应用');
    clickButton('创建网页应用');
    expect(onCreate).toHaveBeenCalledTimes(1);
  });
});
