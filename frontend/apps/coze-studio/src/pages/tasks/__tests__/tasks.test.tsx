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

import { vi } from 'vitest';
import { act } from 'react-dom/test-utils';
import { createRoot, type Root } from 'react-dom/client';

import { workbenchTask } from '@coze-studio/api-schema';

globalThis.IS_REACT_ACT_ENVIRONMENT = true;

const mockUseParams = vi.hoisted(() => vi.fn(() => ({ space_id: 'space-1' })));
const mockNavigate = vi.hoisted(() => vi.fn());
const mockListTasks = vi.hoisted(() => vi.fn());

vi.mock('react-router-dom', () => ({
  useNavigate: () => mockNavigate,
  useParams: mockUseParams,
}));

vi.mock('../service', () => ({
  cancelTask: vi.fn(),
  listTasks: mockListTasks,
  retryTask: vi.fn(),
}));

import {
  canCancelTask,
  filterTasks,
  formatUpdatedTime,
  getTaskStatusText,
} from '../helpers';
import TasksPage from '../index';

describe('TasksPage helpers', () => {
  beforeEach(() => {
    mockUseParams.mockReturnValue({ space_id: 'space-1' });
    mockNavigate.mockReset();
    mockListTasks.mockResolvedValue({
      data: {
        tasks: [
          {
            id: 'task-1',
            space_id: 'space-1',
            creator_id: 'user-1',
            title: '生成周报',
            status: workbenchTask.TaskStatus.Running,
            progress: 40,
            input: '整理项目进展',
            created_at: 1717000000000,
            updated_at: 1717000300000,
          },
        ],
        total: 1,
      },
      code: 0,
      msg: '',
    });
  });

  it('renders the redesigned all tasks structure', async () => {
    const container = document.createElement('div');
    document.body.appendChild(container);
    let root: Root | undefined;

    await act(async () => {
      root = createRoot(container);
      root.render(<TasksPage />);
      await Promise.resolve();
    });

    expect(container.textContent).toContain('全部任务');
    expect(container.textContent).toContain('Aime 专属助理准备好');
    expect(container.textContent).toContain('去聊天专属助理');
    expect(container.textContent).toContain(
      '这里收纳您当前工作空间内的全部任务',
    );
    expect(container.querySelector('input[placeholder="搜索会话"]')).toBeTruthy();
    expect(container.textContent).toContain('已收藏');
    expect(container.textContent).toContain('批量操作');
    expect(container.textContent).toContain('生成周报');
    expect(container.textContent).toContain('运行中');

    act(() => {
      root?.unmount();
    });
    container.remove();
  });

  it('formats backend millisecond timestamps without converting from seconds', () => {
    const timestamp = 1717000300123;

    expect(formatUpdatedTime(timestamp)).toBe(
      new Date(timestamp).toLocaleString(),
    );
  });

  it('allows canceling created tasks', () => {
    expect(canCancelTask(workbenchTask.TaskStatus.Created)).toBe(true);
    expect(canCancelTask(workbenchTask.TaskStatus.Queued)).toBe(true);
    expect(canCancelTask(workbenchTask.TaskStatus.Running)).toBe(true);
  });

  it('filters tasks by title and status', () => {
    const tasks = [
      {
        id: 'task-1',
        space_id: 'space-1',
        creator_id: 'user-1',
        title: '生成周报',
        status: workbenchTask.TaskStatus.Running,
        progress: 40,
        created_at: 1717000000000,
        updated_at: 1717000300000,
      },
      {
        id: 'task-2',
        space_id: 'space-1',
        creator_id: 'user-1',
        title: '排查错误',
        status: workbenchTask.TaskStatus.Failed,
        progress: 100,
        created_at: 1717000000000,
        updated_at: 1717000300000,
      },
    ];

    expect(filterTasks(tasks, '周报', 'all')).toHaveLength(1);
    expect(filterTasks(tasks, '', 'failed')).toHaveLength(1);
    expect(getTaskStatusText(workbenchTask.TaskStatus.Succeeded)).toBe(
      '已完成',
    );
  });
});
