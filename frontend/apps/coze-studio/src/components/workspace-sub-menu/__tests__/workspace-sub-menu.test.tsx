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

const mockNavigate = vi.hoisted(() => vi.fn());
const mockListTaskThreads = vi.hoisted(() => vi.fn());
const mockUseSpaceStore = vi.hoisted(() =>
  vi.fn((selector: (state: { space: { id: string } }) => unknown) =>
    selector({ space: { id: 'space-1' } }),
  ),
);

vi.mock('react-router-dom', () => ({
  useNavigate: () => mockNavigate,
}));

vi.mock('@coze-foundation/space-store', () => ({
  useSpaceStore: mockUseSpaceStore,
}));

vi.mock('../../../pages/tasks/service', () => ({
  listTaskThreads: mockListTaskThreads,
}));

vi.mock('@coze-arch/coze-design', () => {
  const loadingComponent = ({ loading }: { loading: boolean }) =>
    loading ? <span data-testid="loading" /> : null;

  return {
    ['Loading']: loadingComponent,
  };
});

vi.mock('@coze-arch/coze-design/icons', () => {
  const taskIcon = () => <span data-testid="task-icon" />;

  return {
    ['IconCozAsynchronousTask']: taskIcon,
  };
});

import { getWorkspaceTaskStatusMeta } from '../workspace-task-status';
import { WorkspaceTaskList } from '../workspace-task-list';
import { ASSISTANT_BADGE, ASSISTANT_LABEL, WORKSPACE_MENU_META } from '../menu';

describe('Coze Studio WorkspaceSubMenu', () => {
  it('defines the Figma workspace navigation structure', () => {
    const labels = WORKSPACE_MENU_META.map(item => item.label);
    const paths = WORKSPACE_MENU_META.map(item => item.path);

    expect(ASSISTANT_LABEL).toBe('专属助理');
    expect(ASSISTANT_BADGE).toBe('Beta');
    expect(labels).toEqual([
      '新建任务',
      '资源配置',
      '技能配置',
      '开发配置',
      '工具',
      '全部任务',
    ]);
    expect(WORKSPACE_MENU_META[0]).toMatchObject({
      label: '新建任务',
      path: 'chats/new',
      variant: 'primary',
    });
    expect(WORKSPACE_MENU_META.at(-1)).toMatchObject({
      label: '全部任务',
      path: 'chats',
    });
    expect(WORKSPACE_MENU_META[4]).toMatchObject({
      label: '工具',
      path: 'tools',
    });
    expect(paths).not.toContain('task-trigger');
  });

  it('uses distinct sidebar status indicators and keeps green for completed tasks only', () => {
    const completedMeta = getWorkspaceTaskStatusMeta(
      workbenchTask.TaskStatus.Succeeded,
    );
    const runningMeta = getWorkspaceTaskStatusMeta(
      workbenchTask.TaskStatus.Running,
    );
    const failedMeta = getWorkspaceTaskStatusMeta(
      workbenchTask.TaskStatus.Failed,
    );

    expect(completedMeta).toMatchObject({
      tone: 'success',
      color: '#2a9e06',
      ariaLabel: '已完成状态',
    });
    expect(runningMeta).toMatchObject({
      tone: 'running',
      color: '#2a6df4',
      ariaLabel: '运行中状态',
    });
    expect(failedMeta).toMatchObject({
      tone: 'danger',
      color: '#f54a45',
      ariaLabel: '异常状态',
    });
    expect(runningMeta.color).not.toBe(completedMeta.color);
  });

  it('maps canonical task thread statuses to sidebar status indicators', () => {
    expect(getWorkspaceTaskStatusMeta('idle')).toMatchObject({
      tone: 'waiting',
      ariaLabel: '等待状态',
    });
    expect(getWorkspaceTaskStatusMeta('completed')).toMatchObject({
      tone: 'success',
      color: '#2a9e06',
      ariaLabel: '已完成状态',
    });
    expect(getWorkspaceTaskStatusMeta('failed')).toMatchObject({
      tone: 'danger',
      color: '#f54a45',
      ariaLabel: '异常状态',
    });
    expect(getWorkspaceTaskStatusMeta('interrupted')).toMatchObject({
      tone: 'waiting',
      ariaLabel: '等待状态',
    });
  });

  it('renders recent task threads from the canonical task thread source', async () => {
    mockNavigate.mockReset();
    mockListTaskThreads.mockResolvedValue({
      data: {
        threads: [
          {
            thread_id: 'thread-1',
            legacy_task_id: 'task-legacy-1',
            space_id: 'space-1',
            creator_id: 'user-1',
            title: '整理周报',
            status: 'completed',
            source: 'task',
            progress: 100,
            last_user_message: '汇总本周项目进展',
            last_agent_message: '已生成周报',
            created_at: 1717000000000,
            updated_at: 1717000300000,
          },
        ],
        total: 1,
      },
      code: 0,
      msg: '',
    });

    const container = document.createElement('div');
    document.body.appendChild(container);
    let root: Root | undefined;

    await act(async () => {
      root = createRoot(container);
      root.render(<WorkspaceTaskList />);
      await Promise.resolve();
    });

    expect(mockListTaskThreads).toHaveBeenCalledWith({
      space_id: 'space-1',
      page_size: 8,
    });
    expect(container.textContent).toContain('整理周报');

    const taskButton = Array.from(container.querySelectorAll('button')).find(
      button => button.textContent?.includes('整理周报'),
    ) as HTMLButtonElement;

    act(() => {
      taskButton.click();
    });

    expect(mockNavigate).toHaveBeenCalledWith(
      '/space/space-1/chats/task-legacy-1',
    );

    act(() => {
      root?.unmount();
    });
    container.remove();
  });

  it('opens canonical recent task thread when legacy task id is zero string', async () => {
    mockNavigate.mockReset();
    mockListTaskThreads.mockResolvedValue({
      data: {
        threads: [
          {
            thread_id: 'thread-zero-legacy',
            legacy_task_id: '0',
            space_id: 'space-1',
            creator_id: 'user-1',
            title: 'Canonical 新建任务',
            status: 'idle',
            source: 'web',
            progress: 0,
            last_user_message: '请用一句话回复 smoke OK',
            last_agent_message: '',
            created_at: 1717000000000,
            updated_at: 1717000300000,
          },
        ],
        total: 1,
      },
      code: 0,
      msg: '',
    });

    const container = document.createElement('div');
    document.body.appendChild(container);
    let root: Root | undefined;

    await act(async () => {
      root = createRoot(container);
      root.render(<WorkspaceTaskList />);
      await Promise.resolve();
    });

    const taskButton = Array.from(container.querySelectorAll('button')).find(
      button => button.textContent?.includes('Canonical 新建任务'),
    ) as HTMLButtonElement;

    act(() => {
      taskButton.click();
    });

    expect(mockNavigate).toHaveBeenCalledWith(
      '/space/space-1/chats/thread-zero-legacy',
    );

    act(() => {
      root?.unmount();
    });
    container.remove();
  });
});
