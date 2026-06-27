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
import { act, Simulate } from 'react-dom/test-utils';
import { renderToStaticMarkup } from 'react-dom/server';
import { createRoot, type Root } from 'react-dom/client';
import { workbench, workbenchTask } from '@coze-studio/api-schema';

globalThis.IS_REACT_ACT_ENVIRONMENT = true;

const mockUseParams = vi.hoisted(() => vi.fn(() => ({ space_id: 'space-1' })));
const mockNavigate = vi.hoisted(() => vi.fn());
const mockSendWorkbenchChat = vi.hoisted(() => vi.fn());
const mockGetTypeList = vi.hoisted(() => vi.fn());
const mockListSkills = vi.hoisted(() => vi.fn());

vi.mock('react-router-dom', () => ({
  useNavigate: () => mockNavigate,
  useParams: mockUseParams,
}));

vi.mock('../service', () => ({
  sendWorkbenchChat: mockSendWorkbenchChat,
  getWorkbenchLLMModels: mockGetTypeList,
}));

vi.mock('../../skill/service', () => ({
  listSkills: mockListSkills,
}));

/* eslint-disable @typescript-eslint/naming-convention -- Mock exports mirror coze-design component names. */
vi.mock('@coze-arch/coze-design', () => ({
  Button: ({
    'aria-label': ariaLabel,
    children,
    className,
    disabled,
    icon,
    loading,
    onClick,
  }: {
    'aria-label'?: string;
    children: ReactNode;
    className?: string;
    disabled?: boolean;
    icon?: ReactNode;
    loading?: boolean;
    onClick?: () => void;
  }) => (
    <button
      type="button"
      aria-label={ariaLabel}
      className={className}
      disabled={disabled}
      data-loading={loading}
      onClick={onClick}
    >
      {icon}
      {children}
    </button>
  ),
  Input: ({
    'aria-label': ariaLabel,
    className,
    onChange,
    placeholder,
    prefix,
    value,
  }: {
    'aria-label'?: string;
    className?: string;
    onChange?: (value: string) => void;
    placeholder?: string;
    prefix?: ReactNode;
    value?: string;
  }) => (
    <label className={className}>
      {prefix}
      <input
        aria-label={ariaLabel}
        placeholder={placeholder}
        value={value}
        onChange={event => onChange?.(event.target.value)}
      />
    </label>
  ),
  Spin: () => <span>加载中...</span>,
  Tabs: ({
    tabBarExtraContent,
    tabList,
  }: {
    tabBarExtraContent?: ReactNode;
    tabList?: Array<{ itemKey: string; tab: ReactNode }>;
  }) => (
    <div>
      {tabList?.map(item => <span key={item.itemKey}>{item.tab}</span>)}
      {tabBarExtraContent}
    </div>
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
  IconCozArrowDown: () => <span />,
  IconCozAt: () => <span />,
  IconCozBell: () => <span />,
  IconCozImage: () => <span />,
  IconCozLink: () => <span />,
  IconCozPlus: () => <span />,
  IconCozPlugin: () => <span />,
  IconCozSearch: () => <span />,
  IconCozSendFill: () => <span />,
  IconCozSetting: () => <span />,
  IconCozStar: () => <span />,
  IconCozUpload: () => <span />,
}));
/* eslint-enable @typescript-eslint/naming-convention -- Restore naming checks after mocks. */

import WorkbenchPage, { mapModeToChatMode } from '../index';

describe('WorkbenchPage', () => {
  beforeEach(() => {
    mockUseParams.mockReturnValue({ space_id: 'space-1' });
    mockNavigate.mockReset();
    mockSendWorkbenchChat.mockReset();
    mockGetTypeList.mockReset();
    mockListSkills.mockReset();
    mockListSkills.mockResolvedValue({
      data: {
        skills: [
          {
            id: 'skill-101',
            space_id: 'space-1',
            name: 'Research Skill',
            description: 'Research from trusted sources',
            type: 3,
            version: '1.0.0',
            enabled: true,
            input_schema: '{}',
            output_schema: '{}',
            executor: '{}',
            permissions: '{}',
            created_at: 1717000000000,
            updated_at: 1717000300000,
          },
        ],
      },
      code: 0,
      msg: '',
    });
    mockGetTypeList.mockResolvedValue([
      {
        name: 'deepseek-v4-pro',
        model_type: 100002,
        model_class_name: 'DeepSeek',
      },
      {
        name: 'gpt-4.1',
        model_type: 100003,
        model_class_name: 'OpenAI',
      },
    ]);
  });

  it('renders the static chat workbench first screen', () => {
    const markup = renderToStaticMarkup(<WorkbenchPage />);

    expect(markup).toContain('欢迎来到 刘文波 的工作空间');
    expect(markup).toContain('Aime 专属助理准备好,先聊聊吧~');
    expect(markup).toContain('去聊天专属助理');
    expect(markup).toContain('Hi,我会根据你的任务特性,自动匹配最佳的处理方式~');
    expect(markup).toContain('aria-label="任务描述"');
    expect(markup).toContain('Auto');
    expect(markup).toContain('Ask');
    expect(markup).toContain('Agent');
    expect(markup).toContain('aria-label="拓展"');
    expect(markup).toContain('公开模板 6268');
    expect(markup).toContain('我收藏的');
    expect(markup).toContain('我创建的');
    expect(markup).toContain('搜索模板');
    expect(markup).toContain('推荐排序');
    expect(markup).toContain('创建模板');
    expect(markup).toContain('年度工作总结报告(简洁版)');
    expect(markup).toContain('通过代码生成研发年度报告');
    expect(markup).toContain('通用自动化产品 Meego Bug 根因分析与修复');
    expect(markup).toContain('后端架构整体方案设计');
    expect(markup).toContain('Go 专家为你 CodeReview');
    expect(markup).toContain('文档撰写');
    expect(markup).toContain('代码开发');
    expect(markup).toContain('质量检测');
    expect(markup).toContain('服务端');
    expect(markup).toContain('官方');
    expect(markup).toContain('aria-label="发送任务"');
    expect(markup).toContain('aria-pressed="true"');
    expect(markup).toContain('aria-pressed="false"');
  });

  it('switches mode prompt and opens the resource and extension menus from the prototype', async () => {
    const container = document.createElement('div');
    document.body.appendChild(container);
    let root: Root | undefined;

    act(() => {
      root = createRoot(container);
      root.render(<WorkbenchPage />);
    });

    expect(container.textContent).toContain(
      'Hi,我会根据你的任务特性,自动匹配最佳的处理方式~',
    );

    const askButton = Array.from(container.querySelectorAll('button')).find(
      button => button.textContent === 'Ask',
    ) as HTMLButtonElement;
    act(() => {
      askButton.click();
    });

    expect(container.textContent).toContain(
      'Hi,我会以最快的方式自动响应,为你提供高效且清晰的专业答案~',
    );

    const resourceButton = container.querySelector(
      'button[aria-label="添加上下文"]',
    ) as HTMLButtonElement;
    act(() => {
      resourceButton.click();
    });

    expect(container.textContent).toContain('选择资源类型');
    expect(container.textContent).toContain('技能');
    expect(container.textContent).toContain('代码仓库');
    expect(container.textContent).toContain('空间文档库');
    expect(container.textContent).toContain('Esc 退出');

    const extensionButton = Array.from(
      container.querySelectorAll('button'),
    ).find(button => button.textContent?.includes('拓展')) as HTMLButtonElement;
    await act(async () => {
      extensionButton.click();
      await Promise.resolve();
    });

    expect(container.textContent).toContain('技能 1');
    expect(container.textContent).toContain('MCP 0');
    expect(container.textContent).toContain('已选择:');
    expect(container.textContent).toContain('Research Skill');
    expect(mockListSkills).toHaveBeenCalledWith({
      space_id: 'space-1',
      enabled: true,
    });

    const skillConfigButton = Array.from(
      container.querySelectorAll('button'),
    ).find(button =>
      button.textContent?.includes('技能配置'),
    ) as HTMLButtonElement;
    act(() => {
      skillConfigButton.click();
    });

    expect(mockNavigate).toHaveBeenCalledWith('/space/space-1/skill');

    act(() => {
      root?.unmount();
    });
    container.remove();
  });

  it('maps local mode names to generated chat modes', () => {
    expect(mapModeToChatMode('Auto')).toBe(workbench.ChatMode.Auto);
    expect(mapModeToChatMode('Ask')).toBe(workbench.ChatMode.Ask);
    expect(mapModeToChatMode('Agent')).toBe(workbench.ChatMode.Agent);
  });

  it('navigates to the task execution page after send returns a task', async () => {
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
      runtime_settings: expect.any(String),
      enable_skills: [],
      enable_mcp: [],
      enable_kbs: [],
      enable_databases: [],
    });
    expect(
      JSON.parse(mockSendWorkbenchChat.mock.calls[0]?.[0].runtime_settings),
    ).toMatchObject({
      runtime: 'eino_adk',
      memory_retrieval: {
        limit: 5,
        candidate_limit: 20,
        scopes: ['thread', 'long_term'],
        min_confidence: 0.2,
      },
      web_tools: {
        enabled: false,
      },
      token_usage: {
        enabled: true,
      },
    });
    expect(mockNavigate).toHaveBeenCalledWith('/space/space-1/chats/task-1');

    act(() => {
      root?.unmount();
    });
    container.remove();
  });

  it('navigates to the task list without legacy task creation when chat returns no task', async () => {
    const container = document.createElement('div');
    document.body.appendChild(container);
    let root: Root | undefined;

    mockSendWorkbenchChat.mockResolvedValue({
      data: {
        answer: '这是直接回答，不应该留在首页展示。',
        route_target: workbench.RouteTarget.ChatDirect,
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
        target: { value: '帮我写一份报告' },
      } as unknown as Event);
    });

    const sendButton = Array.from(container.querySelectorAll('button')).find(
      button => button.textContent?.includes('发送'),
    ) as HTMLButtonElement;

    await act(async () => {
      sendButton.click();
      await Promise.resolve();
    });

    expect(mockNavigate).toHaveBeenCalledWith('/space/space-1/chats');
    expect(container.textContent).not.toContain(
      '这是直接回答，不应该留在首页展示。',
    );

    act(() => {
      root?.unmount();
    });
    container.remove();
  });

  it('shows the chat error without falling back to legacy task creation', async () => {
    const container = document.createElement('div');
    document.body.appendChild(container);
    let root: Root | undefined;

    mockSendWorkbenchChat.mockRejectedValue(new Error('chat failed'));

    act(() => {
      root = createRoot(container);
      root.render(<WorkbenchPage />);
    });

    const textarea = container.querySelector(
      'textarea[aria-label="任务描述"]',
    ) as HTMLTextAreaElement;
    act(() => {
      Simulate.change(textarea, {
        target: { value: '即使 chat 失败也创建任务' },
      } as unknown as Event);
    });

    const sendButton = Array.from(container.querySelectorAll('button')).find(
      button => button.textContent?.includes('发送'),
    ) as HTMLButtonElement;

    await act(async () => {
      sendButton.click();
      await Promise.resolve();
    });

    expect(mockNavigate).not.toHaveBeenCalledWith(
      '/space/space-1/tasks/task-after-chat-error',
    );
    expect(container.textContent).toContain('chat failed');

    act(() => {
      root?.unmount();
    });
    container.remove();
  });

  it('passes updated skill selections in the chat request', async () => {
    const container = document.createElement('div');
    document.body.appendChild(container);
    let root: Root | undefined;

    mockSendWorkbenchChat.mockResolvedValue({
      data: {
        task: {
          id: 'task-with-skill-selection',
          space_id: 'space-1',
          creator_id: 'user-1',
          title: '执行选中的技能',
          status: workbenchTask.TaskStatus.Running,
          progress: 0,
          created_at: 1717000000,
          updated_at: 1717000000,
        },
      },
      code: 0,
      msg: '',
    });

    act(() => {
      root = createRoot(container);
      root.render(<WorkbenchPage />);
    });

    const extensionButton = Array.from(
      container.querySelectorAll('button'),
    ).find(button => button.textContent?.includes('拓展')) as HTMLButtonElement;
    await act(async () => {
      extensionButton.click();
      await Promise.resolve();
    });

    const skillButton = Array.from(container.querySelectorAll('button')).find(
      button => button.textContent?.includes('Research Skill'),
    ) as HTMLButtonElement;
    act(() => {
      skillButton.click();
    });

    const textarea = container.querySelector(
      'textarea[aria-label="任务描述"]',
    ) as HTMLTextAreaElement;
    act(() => {
      Simulate.change(textarea, {
        target: { value: '执行选中的技能' },
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
      message: '执行选中的技能',
      mode: workbench.ChatMode.Auto,
      model_type: '100002',
      model_name: 'deepseek-v4-pro',
      runtime_settings: expect.any(String),
      enable_skills: ['skill-101'],
      enable_mcp: [],
      enable_kbs: [],
      enable_databases: [],
    });

    act(() => {
      root?.unmount();
    });
    container.remove();
  });

  it('loads workflow llm models and sends the selected model with a new task', async () => {
    const container = document.createElement('div');
    document.body.appendChild(container);
    let root: Root | undefined;

    mockSendWorkbenchChat.mockResolvedValue({
      data: {
        task: {
          id: 'task-with-model-selection',
          space_id: 'space-1',
          creator_id: 'user-1',
          title: '指定模型回答',
          status: workbenchTask.TaskStatus.Running,
          progress: 0,
          created_at: 1717000000,
          updated_at: 1717000000,
        },
      },
      code: 0,
      msg: '',
    });

    await act(async () => {
      root = createRoot(container);
      root.render(<WorkbenchPage />);
      await Promise.resolve();
    });

    expect(mockGetTypeList).toHaveBeenCalledWith('space-1');
    expect(container.textContent).toContain('deepseek-v4-pro');

    const selectorButton = container.querySelector(
      'button[aria-label="选择模型"]',
    ) as HTMLButtonElement;
    act(() => {
      selectorButton.click();
    });

    const modelButton = Array.from(container.querySelectorAll('button')).find(
      button => button.textContent?.includes('gpt-4.1'),
    ) as HTMLButtonElement;
    act(() => {
      modelButton.click();
    });

    const textarea = container.querySelector(
      'textarea[aria-label="任务描述"]',
    ) as HTMLTextAreaElement;
    act(() => {
      Simulate.change(textarea, {
        target: { value: '指定模型回答' },
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
      message: '指定模型回答',
      mode: workbench.ChatMode.Auto,
      model_type: '100003',
      model_name: 'gpt-4.1',
      runtime_settings: expect.any(String),
      enable_skills: [],
      enable_mcp: [],
      enable_kbs: [],
      enable_databases: [],
    });

    act(() => {
      root?.unmount();
    });
    container.remove();
  });

  it('sends model retry and failover runtime settings with a new task', async () => {
    const container = document.createElement('div');
    document.body.appendChild(container);
    let root: Root | undefined;

    mockSendWorkbenchChat.mockResolvedValue({
      data: {
        task: {
          id: 'task-with-model-failover',
          space_id: 'space-1',
          creator_id: 'user-1',
          title: '稳定执行',
          status: workbenchTask.TaskStatus.Running,
          progress: 0,
          created_at: 1717000000,
          updated_at: 1717000000,
        },
      },
      code: 0,
      msg: '',
    });

    await act(async () => {
      root = createRoot(container);
      root.render(<WorkbenchPage />);
      await Promise.resolve();
    });

    const runtimeButton = container.querySelector(
      'button[aria-label="运行设置"]',
    ) as HTMLButtonElement;
    act(() => {
      runtimeButton.click();
    });

    const retryButton = container.querySelector(
      'button[aria-label="模型重试"]',
    ) as HTMLButtonElement;
    const failoverButton = container.querySelector(
      'button[aria-label="模型切换"]',
    ) as HTMLButtonElement;
    act(() => {
      retryButton.click();
      failoverButton.click();
    });

    const textarea = container.querySelector(
      'textarea[aria-label="任务描述"]',
    ) as HTMLTextAreaElement;
    act(() => {
      Simulate.change(textarea, {
        target: { value: '稳定执行' },
      } as unknown as Event);
    });

    const sendButton = Array.from(container.querySelectorAll('button')).find(
      button => button.textContent?.includes('发送'),
    ) as HTMLButtonElement;

    await act(async () => {
      sendButton.click();
      await Promise.resolve();
    });

    const runtimeSettings = JSON.parse(
      mockSendWorkbenchChat.mock.calls[0]?.[0].runtime_settings,
    );
    expect(runtimeSettings).toMatchObject({
      model_retry: {
        max_retries: 1,
        backoff_ms: 0,
        retry_empty_output: true,
        retry_finish_reasons: ['length'],
      },
      model_failover: {
        candidate_model_ids: [100003],
        max_retries: 1,
        failover_empty_output: true,
        failover_finish_reasons: ['length'],
      },
    });

    act(() => {
      root?.unmount();
    });
    container.remove();
  });

  it('sends web fetch runtime settings only after allowed hosts are configured', async () => {
    const container = document.createElement('div');
    document.body.appendChild(container);
    let root: Root | undefined;

    mockSendWorkbenchChat.mockResolvedValue({
      data: {
        task: {
          id: 'task-with-web-fetch',
          space_id: 'space-1',
          creator_id: 'user-1',
          title: '读取网页资料',
          status: workbenchTask.TaskStatus.Running,
          progress: 0,
          created_at: 1717000000,
          updated_at: 1717000000,
        },
      },
      code: 0,
      msg: '',
    });

    await act(async () => {
      root = createRoot(container);
      root.render(<WorkbenchPage />);
      await Promise.resolve();
    });

    const runtimeButton = container.querySelector(
      'button[aria-label="运行设置"]',
    ) as HTMLButtonElement;
    act(() => {
      runtimeButton.click();
    });

    const fetchButton = container.querySelector(
      'button[aria-label="网页读取"]',
    ) as HTMLButtonElement;
    expect(fetchButton.disabled).toBe(true);

    const allowedHostsInput = container.querySelector(
      'input[aria-label="网页读取允许域名"]',
    ) as HTMLInputElement;
    act(() => {
      Simulate.change(allowedHostsInput, {
        target: { value: 'example.com,' },
      } as unknown as Event);
    });
    expect(allowedHostsInput.value).toBe('example.com,');

    expect(fetchButton.disabled).toBe(false);

    act(() => {
      Simulate.change(allowedHostsInput, {
        target: { value: ' example.com, docs.example.com, ' },
      } as unknown as Event);
    });

    act(() => {
      fetchButton.click();
    });

    const textarea = container.querySelector(
      'textarea[aria-label="任务描述"]',
    ) as HTMLTextAreaElement;
    act(() => {
      Simulate.change(textarea, {
        target: { value: '读取网页资料' },
      } as unknown as Event);
    });

    const sendButton = Array.from(container.querySelectorAll('button')).find(
      button => button.textContent?.includes('发送'),
    ) as HTMLButtonElement;

    await act(async () => {
      sendButton.click();
      await Promise.resolve();
    });

    const runtimeSettings = JSON.parse(
      mockSendWorkbenchChat.mock.calls[0]?.[0].runtime_settings,
    );
    expect(runtimeSettings).toMatchObject({
      web_tools: {
        enabled: true,
        http: {
          enabled: true,
          allowed_hosts: ['example.com', 'docs.example.com'],
          timeout_ms: 10000,
          max_response_bytes: 262144,
        },
      },
    });

    act(() => {
      root?.unmount();
    });
    container.remove();
  });
});
