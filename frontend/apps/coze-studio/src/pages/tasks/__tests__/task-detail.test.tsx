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

const mockUseParams = vi.hoisted(() =>
  vi.fn(() => ({ space_id: 'space-1', task_id: 'task-1' })),
);
const mockGetTask = vi.hoisted(() => vi.fn());
const mockListTaskEvents = vi.hoisted(() => vi.fn());

vi.mock('react-router-dom', () => ({
  useParams: mockUseParams,
}));

vi.mock('../service', () => ({
  getTask: mockGetTask,
  listTaskEvents: mockListTaskEvents,
}));

import TaskDetailPage from '../detail';

describe('TaskDetailPage', () => {
  beforeEach(() => {
    mockUseParams.mockReturnValue({ space_id: 'space-1', task_id: 'task-1' });
    mockGetTask.mockResolvedValue({
      data: {
        id: 'task-1',
        space_id: 'space-1',
        creator_id: 'user-1',
        title: '生成周报',
        status: workbenchTask.TaskStatus.Running,
        progress: 65,
        input: JSON.stringify({ message: '请总结本周项目进展' }),
        result: JSON.stringify({ message: '本周完成了 UI 改造方案。' }),
        created_at: 1717000000000,
        updated_at: 1717000300000,
      },
      code: 0,
      msg: '',
    });
    mockListTaskEvents.mockResolvedValue({
      data: {
        events: [
          {
            id: 'event-1',
            task_id: 'task-1',
            event_type: 'created',
            payload: JSON.stringify({ status: 'created' }),
            created_at: 1717000100000,
          },
        ],
      },
      code: 0,
      msg: '',
    });
  });

  it('renders task data and execution events', async () => {
    const container = document.createElement('div');
    document.body.appendChild(container);
    let root: Root | undefined;

    await act(async () => {
      root = createRoot(container);
      root.render(<TaskDetailPage />);
      await Promise.resolve();
    });

    expect(mockGetTask).toHaveBeenCalledWith({ task_id: 'task-1' });
    expect(mockListTaskEvents).toHaveBeenCalledWith({ task_id: 'task-1' });
    expect(container.textContent).toContain('生成周报');
    expect(container.textContent).toContain('Aime · 已为你启动 Agent 工作流');
    expect(container.textContent).toContain('请总结本周项目进展');
    expect(container.textContent).toContain('本周完成了 UI 改造方案。');
    expect(container.textContent).toContain('任务已创建');
    expect(container.textContent).toContain('执行流程');
    expect(container.textContent).not.toContain('{"message":"请总结本周项目进展"}');
    expect(
      container.querySelector('input[placeholder="继续追问..."]'),
    ).toBeTruthy();
    expect(container.textContent).toContain('65%');

    act(() => {
      root?.unmount();
    });
    container.remove();
  });
});
