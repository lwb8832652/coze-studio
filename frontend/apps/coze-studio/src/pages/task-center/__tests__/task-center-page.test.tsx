// Copyright 2025 coze-dev Authors
// SPDX-License-Identifier: Apache-2.0

/* eslint-disable @typescript-eslint/require-await -- Async mocks mirror production contracts. */
/* eslint-disable @typescript-eslint/naming-convention -- Mock exports mirror package component names. */

import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import { act } from 'react-dom/test-utils';
import { createRoot, type Root } from 'react-dom/client';
import { workbenchTask } from '@coze-studio/api-schema';

import type * as TaskCenterService from '../service';

globalThis.IS_REACT_ACT_ENVIRONMENT = true;

const mockListScheduledTasks = vi.hoisted(() => vi.fn());
const mockExecuteScheduledTask = vi.hoisted(() => vi.fn());
const mockEnableScheduledTask = vi.hoisted(() => vi.fn());
const mockDisableScheduledTask = vi.hoisted(() => vi.fn());

vi.mock('react-router-dom', () => ({
  useParams: () => ({ space_id: '200' }),
}));

vi.mock('../../../components/workspace-page-top-bar', () => ({
  WorkspacePageTopBar: () => <div data-testid="workspace-page-top-bar" />,
}));

vi.mock('@coze-arch/coze-design', () => ({
  Modal: ({
    visible,
    title,
    children,
    onCancel,
    onOk,
    okText,
  }: {
    visible?: boolean;
    title?: React.ReactNode;
    children?: React.ReactNode;
    onCancel?: () => void;
    onOk?: () => void;
    okText?: React.ReactNode;
  }) =>
    visible ? (
      <div role="dialog">
        <h2>{title}</h2>
        {children}
        {onOk ? <button onClick={onOk}>{okText || '确定'}</button> : null}
        <button onClick={onCancel}>关闭</button>
      </div>
    ) : null,
  Toast: {
    success: vi.fn(),
    error: vi.fn(),
  },
}));

vi.mock('../service', async importOriginal => {
  const original = await importOriginal<typeof TaskCenterService>();
  return {
    ...original,
    listScheduledTasks: mockListScheduledTasks,
    executeScheduledTask: mockExecuteScheduledTask,
    enableScheduledTask: mockEnableScheduledTask,
    disableScheduledTask: mockDisableScheduledTask,
    listScheduledTaskCronPresets: vi.fn().mockResolvedValue({ data: [] }),
    listScheduledTaskTargets: vi.fn().mockResolvedValue({
      data: { targets: [], total: 0 },
    }),
  };
});

import TaskCenterPage from '../index';

describe('TaskCenterPage', () => {
  let container: HTMLDivElement;
  let root: Root;

  beforeEach(() => {
    container = document.createElement('div');
    document.body.appendChild(container);
    root = createRoot(container);
    mockListScheduledTasks.mockResolvedValue({
      data: {
        tasks: [
          {
            id: '10',
            space_id: '200',
            creator_id: '300',
            name: '每日简报',
            target_type: workbenchTask.ScheduledTaskTargetType.Agent,
            target_id: '400',
            target_name: '信息助理',
            schedule_type: workbenchTask.ScheduledTaskScheduleType.Daily,
            timezone: 'Asia/Shanghai',
            hour: 9,
            minute: 30,
            payload: '{"message":"生成简报"}',
            keep_conversation: true,
            status: workbenchTask.ScheduledTaskStatus.Enabled,
            execution_count: 2,
            max_executions: 0,
            latest_execution_at: 1_720_000_000,
            next_execution_at: 1_720_086_400,
            created_at: 1_719_000_000,
            updated_at: 1_720_000_000,
            version: 1,
          },
        ],
        total: 1,
      },
    });
    mockExecuteScheduledTask.mockResolvedValue({ data: { id: '99' } });
    mockEnableScheduledTask.mockResolvedValue({});
    mockDisableScheduledTask.mockResolvedValue({});
  });

  afterEach(() => {
    act(() => root.unmount());
    container.remove();
    vi.clearAllMocks();
  });

  const renderPage = async () => {
    await act(async () => root.render(<TaskCenterPage />));
    await act(async () => {
      await Promise.resolve();
      await Promise.resolve();
    });
  };

  const clickButton = async (name: string) => {
    const button = Array.from(document.querySelectorAll('button')).find(item =>
      item.textContent?.includes(name),
    );
    expect(button).toBeTruthy();
    await act(async () => {
      button?.dispatchEvent(new MouseEvent('click', { bubbles: true }));
      await Promise.resolve();
      await Promise.resolve();
    });
  };

  it('loads the workspace task list and exposes the full task actions', async () => {
    await renderPage();

    expect(mockListScheduledTasks).toHaveBeenCalledWith({
      space_id: '200',
      page: 1,
      page_size: 20,
    });
    expect(container.textContent).toContain('任务中心');
    expect(container.textContent).toContain('每日简报');
    expect(container.textContent).toContain('信息助理');
    expect(container.textContent).toContain('每天 09:30');
    expect(container.textContent).toContain('执行记录');

    await clickButton('立即执行');
    await act(async () => await Promise.resolve());
    expect(mockExecuteScheduledTask).toHaveBeenCalledWith({
      task_id: '10',
      space_id: '200',
    });

    await clickButton('停用');
    await act(async () => await Promise.resolve());
    expect(mockDisableScheduledTask).toHaveBeenCalledWith({
      task_id: '10',
      space_id: '200',
    });
  });

  it('opens the production task form from the primary action', async () => {
    await renderPage();
    await clickButton('新建任务');

    expect(document.body.textContent).toContain('创建定时任务');
    expect(document.body.textContent).toContain('执行目标');
    expect(document.body.textContent).toContain('调度规则');
    expect(document.body.textContent).toContain('任务内容');
  });
});
