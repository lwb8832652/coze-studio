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

import type { ReactNode } from 'react';

import { vi } from 'vitest';
import { renderToStaticMarkup } from 'react-dom/server';
import { act, Simulate } from 'react-dom/test-utils';
import { createRoot, type Root } from 'react-dom/client';

import { workbench, workbenchTask } from '@coze-studio/api-schema';

globalThis.IS_REACT_ACT_ENVIRONMENT = true;

const mockUseParams = vi.hoisted(() => vi.fn(() => ({ space_id: 'space-1' })));
const mockSendWorkbenchChat = vi.hoisted(() => vi.fn());

vi.mock('react-router-dom', () => ({
  useParams: mockUseParams,
}));

vi.mock('../service', () => ({
  sendWorkbenchChat: mockSendWorkbenchChat,
}));

/* eslint-disable @typescript-eslint/naming-convention -- Mock exports mirror coze-design component names. */
vi.mock('@coze-arch/coze-design', () => ({
  Button: ({
    'aria-label': ariaLabel,
    children,
    disabled,
    icon,
    loading,
    onClick,
  }: {
    'aria-label'?: string;
    children: ReactNode;
    disabled?: boolean;
    icon?: ReactNode;
    loading?: boolean;
    onClick?: () => void;
  }) => (
    <button
      type="button"
      aria-label={ariaLabel}
      disabled={disabled}
      data-loading={loading}
      onClick={onClick}
    >
      {icon}
      {children}
    </button>
  ),
  TextArea: ({
    'aria-label': ariaLabel,
    className,
    onChange,
    placeholder,
    value,
  }: {
    'aria-label'?: string;
    className?: string;
    onChange?: (value: string) => void;
    placeholder?: string;
    value?: string;
  }) => (
    <textarea
      aria-label={ariaLabel}
      className={className}
      placeholder={placeholder}
      value={value}
      onChange={event => onChange?.(event.target.value)}
    />
  ),
}));

vi.mock('@coze-arch/coze-design/icons', () => ({
  IconCozSendFill: () => <span />,
}));
/* eslint-enable @typescript-eslint/naming-convention -- Restore naming checks after mocks. */

import WorkbenchPage, {
  getTaskStatusText,
  mapModeToChatMode,
} from '../index';

describe('WorkbenchPage', () => {
  beforeEach(() => {
    mockUseParams.mockReturnValue({ space_id: 'space-1' });
    mockSendWorkbenchChat.mockReset();
  });

  it('renders the static chat workbench first screen', () => {
    const markup = renderToStaticMarkup(<WorkbenchPage />);

    expect(markup).toContain('欢迎来到 刘文波 的工作空间');
    expect(markup).toContain('让我们一起高效完成工作吧');
    expect(markup).toContain(
      'Hi，我会根据你的任务特性，自动匹配最佳处理方式。',
    );
    expect(markup).toContain('aria-label="任务描述"');
    expect(markup).toContain('Auto');
    expect(markup).toContain('Ask');
    expect(markup).toContain('Agent');
    expect(markup).toContain('选择扩展');
    expect(markup).toContain('研究分析');
    expect(markup).toContain('生成报告');
    expect(markup).toContain('整理知识库');
    expect(markup).toContain('aria-label="发送任务"');
    expect(markup).toContain('aria-pressed="true"');
    expect(markup).toContain('aria-pressed="false"');
  });

  it('maps local mode names to generated chat modes', () => {
    expect(mapModeToChatMode('Auto')).toBe(workbench.ChatMode.Auto);
    expect(mapModeToChatMode('Ask')).toBe(workbench.ChatMode.Ask);
    expect(mapModeToChatMode('Agent')).toBe(workbench.ChatMode.Agent);
  });

  it('renders returned answer and task after send', async () => {
    const container = document.createElement('div');
    document.body.appendChild(container);
    let root: Root | undefined;

    mockSendWorkbenchChat.mockResolvedValue({
      data: {
        answer: '可以，我会处理这项任务。',
        task: {
          id: 'task-1',
          space_id: 'space-1',
          creator_id: 'user-1',
          title: '生成周报',
          status: workbenchTask.TaskStatus.Running,
          progress: 35,
          created_at: 1717000000,
          updated_at: 1717000300,
        },
      },
      code: 0,
      msg: '',
    });

    act(() => {
      root = createRoot(container);
      root.render(<WorkbenchPage />);
    });

    const textarea = container.querySelector(
      'textarea[aria-label="任务描述"]',
    ) as HTMLTextAreaElement;
    act(() => {
      Simulate.change(textarea, {
        target: { value: '帮我生成周报' },
      } as unknown as Event);
    });

    const sendButton = Array.from(container.querySelectorAll('button')).find(
      button => button.textContent?.includes('发送'),
    ) as HTMLButtonElement;

    await act(async () => {
      sendButton.click();
      await Promise.resolve();
    });

    expect(mockSendWorkbenchChat).toHaveBeenCalledWith({
      space_id: 'space-1',
      message: '帮我生成周报',
      mode: workbench.ChatMode.Auto,
    });
    expect(container.textContent).toContain('可以，我会处理这项任务。');
    expect(container.textContent).toContain('生成周报');
    expect(container.textContent).toContain(
      getTaskStatusText(workbenchTask.TaskStatus.Running),
    );
    expect(container.textContent).toContain('35%');

    act(() => {
      root?.unmount();
    });
    container.remove();
  });
});
