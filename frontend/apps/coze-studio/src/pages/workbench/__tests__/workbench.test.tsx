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

/* eslint-disable @typescript-eslint/naming-convention, @typescript-eslint/consistent-type-imports, @typescript-eslint/require-await, @typescript-eslint/no-shadow -- Test doubles mirror browser and asynchronous SDK contracts. */

import type { KeyboardEvent, ReactNode } from 'react';

import { afterAll, afterEach, vi } from 'vitest';
import { act, Simulate } from 'react-dom/test-utils';
import { renderToStaticMarkup } from 'react-dom/server';
import { createRoot as createReactRoot, type Root } from 'react-dom/client';

const originalActEnvironment = globalThis.IS_REACT_ACT_ENVIRONMENT;
globalThis.IS_REACT_ACT_ENVIRONMENT = true;
const originalGetBoundingClientRect =
  HTMLElement.prototype.getBoundingClientRect;
const testRoots = new Set<Root>();

const createRoot = (...args: Parameters<typeof createReactRoot>): Root => {
  const reactRoot = createReactRoot(...args);
  let mounted = true;
  const trackedRoot: Root = {
    render: children => {
      reactRoot.render(children);
    },
    unmount: () => {
      if (!mounted) {
        return;
      }
      mounted = false;
      testRoots.delete(trackedRoot);
      reactRoot.unmount();
    },
  };
  testRoots.add(trackedRoot);

  return trackedRoot;
};

const mockUseParams = vi.hoisted(() => vi.fn(() => ({ space_id: 'space-1' })));
const mockUseLocation = vi.hoisted(() => vi.fn(() => ({ state: null })));
const mockNavigate = vi.hoisted(() => vi.fn());
const mockCreateTaskThread = vi.hoisted(() => vi.fn());
const mockCreateTaskThreadRun = vi.hoisted(() => vi.fn());
const mockUploadTaskThreadFiles = vi.hoisted(() => vi.fn());
const mockGetTypeList = vi.hoisted(() => vi.fn());
const mockListKnowledgeResources = vi.hoisted(() => vi.fn());
const mockListDatabaseResources = vi.hoisted(() => vi.fn());
const mockListWorkflowResources = vi.hoisted(() => vi.fn());
const mockListSkills = vi.hoisted(() => vi.fn());
const mockListMCPToolRegistryEntries = vi.hoisted(() => vi.fn());
const mockWorkspaceHeaderActionsRender = vi.hoisted(() => vi.fn());

vi.mock('react-router-dom', () => ({
  useNavigate: () => mockNavigate,
  useLocation: mockUseLocation,
  useParams: mockUseParams,
  useSearchParams: () => [new URLSearchParams(), vi.fn()],
}));

vi.mock('@coze-arch/foundation-sdk', async importOriginal => {
  const actual =
    await importOriginal<typeof import('@coze-arch/foundation-sdk')>();
  return {
    ...actual,
    getIsLogined: () => true,
    getIsSettled: () => true,
    getUserAuthInfos: async () => undefined,
    getUserInfo: () => ({
      name: '刘文波',
      screen_name: 'wb',
      email: '840582614@qq.com',
      avatar_url: '',
    }),
    subscribeUserAuthInfos: () => () => undefined,
    useIsLogined: () => true,
    useIsSettled: () => true,
    useUserAuthInfo: () => undefined,
    useUserInfo: () => ({
      name: '刘文波',
      screen_name: 'wb',
      email: '840582614@qq.com',
      avatar_url: '',
    }),
    useUserLabel: () => undefined,
  };
});

vi.mock('@coze-foundation/global-adapter/account-settings', () => ({
  useAccountSettings: () => ({
    node: null,
    open: vi.fn(),
  }),
}));

vi.mock('../service', () => ({
  createTaskThread: mockCreateTaskThread,
  createTaskThreadRun: mockCreateTaskThreadRun,
  getWorkbenchLLMModels: mockGetTypeList,
  listWorkbenchDatabaseResources: mockListDatabaseResources,
  listWorkbenchKnowledgeResources: mockListKnowledgeResources,
  listWorkbenchWorkflowResources: mockListWorkflowResources,
  uploadTaskThreadFiles: mockUploadTaskThreadFiles,
}));

vi.mock('../../skill/service', () => ({
  listSkills: mockListSkills,
}));

vi.mock('../../tools/service', () => ({
  listMCPToolRegistryEntries: mockListMCPToolRegistryEntries,
}));

vi.mock('../../../components/workspace-header-actions', () => ({
  WorkspaceHeaderActions: () => {
    mockWorkspaceHeaderActionsRender();
    return <div aria-label="工作区头部账号与通知操作" />;
  },
}));

/* eslint-disable @typescript-eslint/naming-convention -- Mock exports mirror coze-design component names. */
vi.mock('@coze-arch/coze-design', () => ({
  Avatar: ({
    'aria-label': ariaLabel,
    children,
    className,
    src,
    style,
  }: {
    'aria-label'?: string;
    children?: ReactNode;
    className?: string;
    src?: string;
    style?: Record<string, string>;
  }) => (
    <span
      aria-label={ariaLabel}
      className={className}
      data-avatar-src={src}
      style={style}
    >
      {children}
    </span>
  ),
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
    onChange,
    tabBarExtraContent,
    tabList,
  }: {
    onChange?: (key: string) => void;
    tabBarExtraContent?: ReactNode;
    tabList?: Array<{ itemKey: string; tab: ReactNode }>;
  }) => (
    <div>
      {tabList?.map(item => (
        <button
          key={item.itemKey}
          type="button"
          onClick={() => onChange?.(item.itemKey)}
        >
          {item.tab}
        </button>
      ))}
      {tabBarExtraContent}
    </div>
  ),
  TextArea: ({
    'aria-label': ariaLabel,
    className,
    disabled,
    onBlur,
    onChange,
    onCompositionEnd,
    onCompositionStart,
    onFocus,
    onKeyDown,
    placeholder,
    readOnly,
    value,
  }: {
    'aria-label'?: string;
    className?: string;
    disabled?: boolean;
    onBlur?: () => void;
    onChange?: (value: string) => void;
    onCompositionEnd?: React.CompositionEventHandler<HTMLTextAreaElement>;
    onCompositionStart?: React.CompositionEventHandler<HTMLTextAreaElement>;
    onFocus?: () => void;
    onKeyDown?: (event: KeyboardEvent<HTMLTextAreaElement>) => void;
    placeholder?: string;
    readOnly?: boolean;
    value?: string;
  }) => (
    <textarea
      aria-label={ariaLabel}
      className={className}
      disabled={disabled}
      placeholder={placeholder}
      readOnly={readOnly}
      value={value}
      onBlur={onBlur}
      onChange={event => onChange?.(event.target.value)}
      onCompositionEnd={onCompositionEnd}
      onCompositionStart={onCompositionStart}
      onFocus={onFocus}
      onKeyDown={onKeyDown}
    />
  ),
}));

vi.mock('@coze-arch/coze-design/icons', () => ({
  IconCozArrowUp: () => <span />,
  IconCozArrowUpFill: () => <span />,
  IconCozArrowBack: () => <span />,
  IconCozArrowDown: () => <span />,
  IconCozArrowRight: () => <span />,
  IconCozAt: () => <span />,
  IconCozBell: () => <span />,
  IconCozBot: () => <span />,
  IconCozCode: () => <span />,
  IconCozCheckMark: () => <span />,
  IconCozCross: () => <span />,
  IconCozDatabase: () => <span />,
  IconCozDiamondFill: () => <span />,
  IconCozDocument: () => <span />,
  IconCozImage: () => <span />,
  IconCozLink: () => <span />,
  IconCozLightbulb: () => <span />,
  IconCozLightbulbFill: () => <span />,
  IconCozLightningFill: () => <span />,
  IconCozKnowledge: () => <span />,
  IconCozPlus: () => <span />,
  IconCozPlugin: () => <span />,
  IconCozSearch: () => <span />,
  IconCozMagnifier: () => <span />,
  IconCozRocketFill: () => <span />,
  IconCozSendFill: () => <span />,
  IconCozSetting: () => <span />,
  IconCozSkill: () => <span />,
  IconCozStar: () => <span />,
  IconCozStopCircle: () => <span />,
  IconCozUpload: () => <span />,
  IconCozWorkflow: () => <span />,
}));
/* eslint-enable @typescript-eslint/naming-convention -- Restore naming checks after mocks. */

import WorkbenchPage, { WorkbenchTopbar } from '../index';
import { WorkbenchComposer } from '../components/workbench-composer';
import {
  createDefaultWorkbenchResourceSelection,
  createDefaultWorkbenchRuntimeSettings,
  createWorkbenchRunConfig,
} from '../components/types';

const buildCreateTaskThreadResponse = (threadId: string, title: string) => ({
  data: {
    thread: {
      thread_id: threadId,
      space_id: 'space-1',
      creator_id: 'user-1',
      title,
      status: 'running',
      source: 'web',
      progress: 0,
      last_user_message: title,
      last_agent_message: '',
      created_at: 1717000000,
      updated_at: 1717000000,
    },
    message: {
      message_id: `${threadId}-message`,
      thread_id: threadId,
      run_id: `${threadId}-run`,
      role: 'user',
      content: title,
      metadata: '{}',
      created_at: 1717000000,
    },
    run: {
      run_id: `${threadId}-run`,
      thread_id: threadId,
      parent_run_id: '0',
      space_id: 'space-1',
      creator_id: 'user-1',
      assistant_id: 'default',
      run_kind: 'task',
      status: 'pending',
      command: '{}',
      input: '{}',
      config: '{}',
      context: '{}',
      metadata: '{}',
      stream_mode: '["messages","updates"]',
      multitask_strategy: 'enqueue',
      on_disconnect: 'continue',
      durability: 'async',
      idempotency_key: '',
      worker_id: '',
      error_code: '',
      error_message: '',
      started_at: 0,
      ended_at: 0,
      created_at: 1717000000,
      updated_at: 1717000000,
    },
  },
  code: 0,
  msg: '',
});

const getSendButton = (container: HTMLElement) => {
  const sendButton = container.querySelector(
    'button[aria-label="发送任务"]',
  ) as HTMLButtonElement | null;

  expect(sendButton).toBeTruthy();

  return sendButton!;
};

const createDOMRectMock = ({
  height,
  left,
  top,
  width,
}: {
  height: number;
  left: number;
  top: number;
  width: number;
}) =>
  ({
    bottom: top + height,
    height,
    left,
    right: left + width,
    toJSON: () => ({}),
    top,
    width,
    x: left,
    y: top,
  }) as DOMRect;

const installWorkbenchAtAnchorRectMock = ({
  anchorLeft = 264,
  composerLeft = 100,
  composerWidth = 920,
}: {
  anchorLeft?: number;
  composerLeft?: number;
  composerWidth?: number;
} = {}) => {
  const originalGetBoundingClientRect =
    HTMLElement.prototype.getBoundingClientRect;

  HTMLElement.prototype.getBoundingClientRect =
    function getBoundingClientRect() {
      if (this.classList.contains('chat-workbench-composer')) {
        return createDOMRectMock({
          height: 180,
          left: composerLeft,
          top: 120,
          width: composerWidth,
        });
      }

      if (this.classList.contains('chat-workbench-at-prefix')) {
        return createDOMRectMock({
          height: 20,
          left: anchorLeft,
          top: 152,
          width: 8,
        });
      }

      return originalGetBoundingClientRect.call(this);
    };

  return () => {
    HTMLElement.prototype.getBoundingClientRect = originalGetBoundingClientRect;
  };
};

afterEach(() => {
  for (const root of [...testRoots]) {
    act(() => {
      root.unmount();
    });
  }
  testRoots.clear();
  document.body.replaceChildren();
  HTMLElement.prototype.getBoundingClientRect = originalGetBoundingClientRect;
  vi.restoreAllMocks();
});

afterAll(() => {
  globalThis.IS_REACT_ACT_ENVIRONMENT = originalActEnvironment;
});

describe('WorkbenchPage', () => {
  beforeEach(() => {
    window.localStorage.clear();
    mockUseParams.mockReturnValue({ space_id: 'space-1' });
    mockUseLocation.mockReturnValue({ state: null });
    mockNavigate.mockReset();
    mockCreateTaskThread.mockReset();
    mockCreateTaskThreadRun.mockReset();
    mockUploadTaskThreadFiles.mockReset();
    mockGetTypeList.mockReset();
    mockWorkspaceHeaderActionsRender.mockClear();
    mockListKnowledgeResources.mockReset();
    mockListKnowledgeResources.mockResolvedValue([
      {
        id: 'kb-101',
        name: 'Product Knowledge',
        description: 'Product docs',
      },
    ]);
    mockListDatabaseResources.mockReset();
    mockListDatabaseResources.mockResolvedValue([
      {
        id: 'db-101',
        name: 'Orders Database',
        description: 'Order records',
      },
    ]);
    mockListWorkflowResources.mockReset();
    mockListWorkflowResources.mockResolvedValue([
      {
        id: 'workflow-101',
        name: 'Issue Triage Workflow',
        description: 'Triage incoming issues',
      },
    ]);
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
    mockListMCPToolRegistryEntries.mockReset();
    mockListMCPToolRegistryEntries.mockResolvedValue({
      data: {
        tools: [
          {
            name: 'mcp_7656806170115440640_search_repositories',
            source: 'mcp',
            category: 'mcp',
            visibility: 'deferred',
            server_id: '7656806170115440640',
            server_name: 'github',
            tool_name: 'search_repositories',
            description: 'Search for GitHub repositories',
            input_schema: '{"type":"object"}',
            enabled: true,
            health_status: 'unknown',
            health_checked_at: 0,
            health_latency_ms: 0,
            health_error: '',
          },
          {
            name: 'mcp_7656806170115440640_create_issue',
            source: 'mcp',
            category: 'mcp',
            visibility: 'deferred',
            server_id: '7656806170115440640',
            server_name: 'github',
            tool_name: 'create_issue',
            description: 'Create a GitHub issue',
            input_schema: '{"type":"object"}',
            enabled: true,
            health_status: 'unknown',
            health_checked_at: 0,
            health_latency_ms: 0,
            health_error: '',
          },
          {
            name: 'mcp_7656806170694254592_query',
            source: 'mcp',
            category: 'mcp',
            visibility: 'deferred',
            server_id: '7656806170694254592',
            server_name: 'postgres',
            tool_name: 'query',
            description: 'Run a read-only SQL query',
            input_schema: '{"type":"object"}',
            enabled: true,
            health_status: 'unknown',
            health_checked_at: 0,
            health_latency_ms: 0,
            health_error: '',
          },
        ],
        total: 2,
      },
      code: 0,
      msg: '',
    });
    mockGetTypeList.mockResolvedValue([
      {
        name: 'deepseek-v4-pro',
        model_name: 'deepseek-v4-pro',
        model_type: 100002,
        model_class_name: 'DeekSeek',
      },
      {
        name: 'gpt-4.1',
        model_name: 'gpt-4.1',
        model_type: 100003,
        model_class_name: 'OpenAI',
      },
    ]);
  });

  it('navigates the assistant shortcut to the current workspace chats', () => {
    const container = document.createElement('div');
    document.body.appendChild(container);
    const root = createRoot(container);

    act(() => {
      root.render(<WorkbenchTopbar />);
    });

    const assistantButton = Array.from(
      container.querySelectorAll('button'),
    ).find(button => button.textContent === '去聊天专属助理');

    act(() => {
      assistantButton?.click();
    });

    expect(mockNavigate).toHaveBeenCalledWith('/space/space-1/chats');
    expect(mockWorkspaceHeaderActionsRender).toHaveBeenCalledTimes(1);

    act(() => {
      root.unmount();
    });
    container.remove();
  });

  it('disables the assistant shortcut without a workspace id', () => {
    mockUseParams.mockReturnValue({ space_id: '' });
    const container = document.createElement('div');
    document.body.appendChild(container);
    const root = createRoot(container);

    act(() => {
      root.render(<WorkbenchTopbar />);
    });

    const assistantButton = Array.from(
      container.querySelectorAll('button'),
    ).find(button => button.textContent === '去聊天专属助理');

    expect(assistantButton?.disabled).toBe(true);

    act(() => {
      assistantButton?.click();
    });

    expect(mockNavigate).not.toHaveBeenCalled();

    act(() => {
      root.unmount();
    });
    container.remove();
  });

  it('renders the static chat workbench first screen', () => {
    const markup = renderToStaticMarkup(<WorkbenchPage />);

    expect(markup).toContain('欢迎回来，刘文波');
    expect(markup).toContain('NewX AI 专属助理已就绪，随时可以开始对话');
    expect(markup).toContain('去聊天专属助理');
    expect(markup).toContain('aria-label="工作区头部账号与通知操作"');
    expect(markup).toContain('aria-label="任务描述"');
    expect(markup).toContain('data-variant="hero"');
    expect(markup).toContain('data-composer-style="deerflow"');
    expect(markup).toContain('placeholder="今天想做什么？"');
    expect(markup).not.toContain('chat-workbench-deerflow-mode-trigger');
    expect(markup).toContain('chat-workbench-send-deerflow');
    expect(markup).not.toContain('选择模式');
    expect(markup).not.toContain('Auto');
    expect(markup).not.toContain('Ask');
    expect(markup).not.toContain('>Agent<');
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
    expect(markup).toContain('生成 agent 学习路线');
    expect(markup).toContain('chat-workbench-template-icon');
    expect(markup).toContain('chat-workbench-template-create-actions');
    expect(markup).not.toContain('chat-workbench-create-card');
    expect(markup).toContain('文档撰写');
    expect(markup).toContain('代码开发');
    expect(markup).toContain('质量检测');
    expect(markup).toContain('服务端');
    expect(markup).toContain('官方');
    expect(markup).toContain('aria-label="发送任务"');
    expect(markup).not.toContain('aria-pressed="true"');
    expect(markup).not.toContain('aria-pressed="false"');
  });

  it('prefills the shared hero composer from a workbench template', () => {
    const container = document.createElement('div');
    document.body.appendChild(container);
    const root = createRoot(container);

    act(() => {
      root.render(<WorkbenchPage />);
    });
    act(() => {
      (
        container.querySelector(
          'button[aria-label="年度工作总结报告(简洁版) 模板"]',
        ) as HTMLButtonElement
      ).click();
    });

    const textarea = container.querySelector(
      'textarea[aria-label="任务描述"]',
    ) as HTMLTextAreaElement;
    expect(textarea.value).toBe(
      '帮我生成一份年度工作总结报告，要求结构清晰、简洁专业。',
    );
    expect(
      textarea.closest('.chat-composer[data-variant="hero"]'),
    ).not.toBeNull();

    act(() => {
      root.unmount();
    });
    container.remove();
  });

  it('renders skill creation intent in the shared workbench home', () => {
    mockUseLocation.mockReturnValue({
      state: {
        workbenchIntent: 'create_skill',
        initialMessage:
          '我想创建一个技能，请先询问我技能用途、使用场景和期望输出。',
      },
    });

    const markup = renderToStaticMarkup(<WorkbenchPage />);

    expect(markup).toContain('欢迎回来，刘文波');
    expect(markup).toContain('AI 创建技能');
    expect(markup).toContain('描述你想创建的技能、使用场景和期望输出');
    expect(markup).toContain(
      '我想创建一个技能，请先询问我技能用途、使用场景和期望输出。',
    );
    expect(markup).toContain('placeholder="今天想做什么？"');
    expect(markup).toContain('data-variant="hero"');
    expect(markup).toContain('公开模板 6268');
    expect(markup).not.toContain('✨ 创建你自己的 Agent SKill ✨');
    expect(markup).not.toContain(
      '创建你的 Agent Skill 来释放 DeerFlow 的潜力。',
    );
    expect(markup).not.toContain('第一步请先直接问我');
    expect(markup).not.toContain('不要创建文件或生成 .skill 包');
  });

  it('activates skill-creator when sending from DeerFlow skill-creator mode', async () => {
    mockUseLocation.mockReturnValue({
      state: {
        workbenchIntent: 'create_skill',
        initialMessage:
          '我想创建一个技能，请先询问我技能用途、使用场景和期望输出。',
      },
    });
    mockCreateTaskThread.mockResolvedValue(
      buildCreateTaskThreadResponse(
        'thread-skill-creator-mode',
        '创建一个技能',
      ),
    );

    const container = document.createElement('div');
    document.body.appendChild(container);
    let root: Root | undefined;

    await act(async () => {
      root = createRoot(container);
      root.render(<WorkbenchPage />);
      await Promise.resolve();
    });

    const textarea = container.querySelector(
      'textarea[aria-label="任务描述"]',
    ) as HTMLTextAreaElement;
    act(() => {
      Simulate.change(textarea, {
        target: { value: '创建一个项目周报技能' },
      } as unknown as Event);
    });

    const sendButton = getSendButton(container);
    await act(async () => {
      sendButton.click();
      await Promise.resolve();
    });

    expect(mockCreateTaskThread).toHaveBeenCalledWith({
      space_id: 'space-1',
      message: '创建一个项目周报技能',
      config: expect.any(String),
    });
    expect(
      JSON.parse(mockCreateTaskThread.mock.calls[0]?.[0].config),
    ).toMatchObject({
      enable_skills: ['skill-creator'],
      skills: {
        enabled: true,
        allowed_skills: ['skill-creator'],
      },
    });

    act(() => {
      root?.unmount();
    });
    container.remove();
  });

  it('opens resource and extension menus from the prototype', async () => {
    const container = document.createElement('div');
    document.body.appendChild(container);
    let root: Root | undefined;

    act(() => {
      root = createRoot(container);
      root.render(<WorkbenchPage />);
    });

    expect(
      container.querySelector('.chat-workbench-deerflow-mode-trigger'),
    ).toBeNull();

    const resourceButton = container.querySelector(
      'button[aria-label="添加上下文"]',
    ) as HTMLButtonElement;
    act(() => {
      resourceButton.click();
    });

    expect(container.textContent).toContain('选择资源类型');
    expect(container.textContent).toContain('技能');
    expect(container.textContent).toContain('知识库');
    expect(container.textContent).toContain('数据库');
    expect(container.textContent).toContain('工作流');
    expect(container.textContent).toContain('插件');
    expect(container.textContent).toContain('代码仓库');
    expect(container.textContent).not.toContain('仓库分支');
    expect(container.textContent).not.toContain('代码文件夹');
    expect(container.textContent).not.toContain('空间文档库');
    expect(container.textContent).toContain('Esc 退出');

    const extensionButton = Array.from(
      container.querySelectorAll('button'),
    ).find(button => button.textContent?.includes('拓展')) as HTMLButtonElement;
    await act(async () => {
      extensionButton.click();
      await Promise.resolve();
      await Promise.resolve();
    });

    expect(container.textContent).toContain('技能 1');
    expect(container.textContent).toContain('MCP 2');
    expect(container.textContent).toContain('已启用 3/3');
    expect(container.textContent).not.toContain('已选择:');
    expect(container.textContent).toContain('Research Skill');
    expect(mockListSkills).toHaveBeenCalledWith({
      space_id: 'space-1',
      enabled: true,
    });
    expect(mockListMCPToolRegistryEntries).toHaveBeenCalledWith({
      space_id: 'space-1',
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

  it('renders NewX AI @ references as inline resource tokens in the DeerFlow composer', async () => {
    const container = document.createElement('div');
    document.body.appendChild(container);
    const restoreRects = installWorkbenchAtAnchorRectMock();
    let root: Root | undefined;

    act(() => {
      root = createRoot(container);
      root.render(<WorkbenchPage />);
    });

    const textarea = container.querySelector(
      'textarea[aria-label="任务描述"]',
    ) as HTMLTextAreaElement;
    await act(async () => {
      Simulate.change(textarea, { target: { value: '请参考@' } });
      await Promise.resolve();
    });

    const resourceInline = container.querySelector('.chat-workbench-at-inline');
    expect(resourceInline?.textContent).toContain('@');
    expect(container.textContent).toContain('选择资源类型');
    expect(container.textContent).toContain('请参考');
    const atMenu = container.querySelector(
      '.chat-workbench-at-menu',
    ) as HTMLElement;
    expect(atMenu?.dataset.placement).toBe('bottom');
    expect(atMenu?.style.left).toBe('164px');
    expect(atMenu?.style.top).toBe('56px');

    const resourceSearch = container.querySelector(
      'input[aria-label="@资源搜索"]',
    ) as HTMLInputElement;
    expect(resourceSearch?.placeholder).toBe('选择资源类型');
    await act(async () => {
      Simulate.change(resourceSearch, { target: { value: '技能' } });
      await Promise.resolve();
    });
    expect(container.textContent).toContain('技能');
    expect(container.textContent).not.toContain('代码仓库');

    const skillResourceButton = Array.from(
      container.querySelectorAll('.chat-workbench-at-menu-list button'),
    ).find(button => button.textContent?.includes('技能')) as HTMLButtonElement;
    await act(async () => {
      skillResourceButton.click();
      await Promise.resolve();
      await Promise.resolve();
    });

    const skillInline = container.querySelector('.chat-workbench-at-inline');
    expect(skillInline?.textContent).toContain('@技能：');

    const skillSearch = container.querySelector(
      'input[aria-label="@技能搜索"]',
    ) as HTMLInputElement;
    expect(skillSearch?.placeholder).toBe('请输入搜索技能');
    await act(async () => {
      Simulate.change(skillSearch, { target: { value: 'Research' } });
      await Promise.resolve();
    });
    expect(container.textContent).toContain('Research Skill');

    const skillButton = Array.from(
      container.querySelectorAll('.chat-workbench-at-menu-list button'),
    ).find(button =>
      button.textContent?.includes('Research Skill'),
    ) as HTMLButtonElement;
    await act(async () => {
      skillButton.click();
      await Promise.resolve();
    });

    const referenceChip = container.querySelector(
      '.chat-workbench-at-resource-chip',
    );
    expect(
      referenceChip?.querySelector('.chat-workbench-at-resource-icon span'),
    ).not.toBeNull();
    expect(referenceChip?.textContent).toContain('Research Skill');
    expect(referenceChip?.textContent).not.toContain('@');
    expect(referenceChip?.textContent).not.toContain('技能：');
    expect(container.querySelector('.chat-workbench-at-inline')).toBeNull();
    expect(container.querySelector('.chat-workbench-at-menu')).toBeNull();
    expect(document.activeElement).toBe(textarea);

    await act(async () => {
      Simulate.change(textarea, { target: { value: '继续处理@' } });
      await Promise.resolve();
    });
    const secondSkillResourceButton = Array.from(
      container.querySelectorAll('.chat-workbench-at-menu-list button'),
    ).find(button => button.textContent?.includes('技能')) as HTMLButtonElement;
    await act(async () => {
      secondSkillResourceButton.click();
      await Promise.resolve();
      await Promise.resolve();
    });
    const secondSkillSearch = container.querySelector(
      'input[aria-label="@技能搜索"]',
    ) as HTMLInputElement;
    await act(async () => {
      Simulate.change(secondSkillSearch, { target: { value: 'Research' } });
      await Promise.resolve();
    });
    const secondSkillButton = Array.from(
      container.querySelectorAll('.chat-workbench-at-menu-list button'),
    ).find(button =>
      button.textContent?.includes('Research Skill'),
    ) as HTMLButtonElement;
    act(() => {
      secondSkillButton.click();
    });

    const referenceChips = container.querySelectorAll(
      '.chat-workbench-at-resource-chip',
    );
    expect(referenceChips).toHaveLength(2);
    expect(container.textContent).toContain('继续处理');

    await act(async () => {
      Simulate.keyDown(textarea, { key: 'Backspace' });
      await Promise.resolve();
    });
    expect(
      container.querySelectorAll('.chat-workbench-at-resource-chip'),
    ).toHaveLength(1);
    expect(container.textContent).toContain('请参考');
    expect(container.textContent).toContain('继续处理');
    expect(document.activeElement).toBe(textarea);

    act(() => {
      root?.unmount();
    });
    restoreRects();
    container.remove();
  });

  it('anchors the NewX AI @ resource menu above the inline marker in the detail composer', async () => {
    const container = document.createElement('div');
    document.body.appendChild(container);
    const restoreRects = installWorkbenchAtAnchorRectMock();
    let root: Root | undefined;

    act(() => {
      root = createRoot(container);
      root.render(
        <WorkbenchComposer
          value=""
          loading={false}
          variant="detail"
          presentation="deerflow"
          spaceId="space-1"
          onValueChange={vi.fn()}
          onSubmit={vi.fn()}
        />,
      );
    });

    const textarea = container.querySelector(
      'textarea[aria-label="任务描述"]',
    ) as HTMLTextAreaElement;
    await act(async () => {
      Simulate.change(textarea, { target: { value: '@' } });
      await Promise.resolve();
    });

    const atMenu = container.querySelector(
      '.chat-workbench-at-menu',
    ) as HTMLElement;
    expect(atMenu?.dataset.placement).toBe('top');
    expect(atMenu?.style.left).toBe('164px');
    expect(atMenu?.style.bottom).toBe('152px');

    act(() => {
      root?.unmount();
    });
    restoreRects();
    container.remove();
  });

  it('opens the NewX AI @ resource reference flow when typing @ in the composer', async () => {
    const container = document.createElement('div');
    document.body.appendChild(container);
    let root: Root | undefined;

    act(() => {
      root = createRoot(container);
      root.render(<WorkbenchPage />);
    });

    const textarea = container.querySelector(
      'textarea[aria-label="任务描述"]',
    ) as HTMLTextAreaElement;
    await act(async () => {
      Simulate.change(textarea, { target: { value: '@' } });
      await Promise.resolve();
    });

    expect(textarea.value).toBe('');
    expect(container.textContent).toContain('@');
    expect(container.textContent).toContain('选择资源类型');
    expect(
      container.querySelector('input[aria-label="@资源搜索"]'),
    ).toBeTruthy();

    await act(async () => {
      Simulate.keyDown(
        container.querySelector(
          'input[aria-label="@资源搜索"]',
        ) as HTMLInputElement,
        {
          key: 'Backspace',
        },
      );
      await Promise.resolve();
    });

    expect(container.querySelector('.chat-workbench-at-inline')).toBeNull();
    expect(container.querySelector('.chat-workbench-at-menu')).toBeNull();
    expect(textarea.value).toBe('');
    expect(document.activeElement).toBe(textarea);

    act(() => {
      root?.unmount();
    });
    container.remove();
  });

  it('keeps selected attachments in the composer payload', async () => {
    const container = document.createElement('div');
    document.body.appendChild(container);
    let root: Root | undefined;
    const onSubmit = vi.fn();
    const file = new File(['hello'], 'brief.md', { type: 'text/markdown' });

    act(() => {
      root = createRoot(container);
      root.render(
        <WorkbenchComposer
          value="请总结附件"
          loading={false}
          presentation="deerflow"
          spaceId="space-1"
          onValueChange={vi.fn()}
          onSubmit={onSubmit}
        />,
      );
    });

    const uploadButton = container.querySelector(
      'button[aria-label="添加附件"]',
    ) as HTMLButtonElement;
    const fileInput = container.querySelector(
      'input[type="file"]',
    ) as HTMLInputElement;

    await act(async () => {
      uploadButton.click();
      Simulate.change(fileInput, {
        target: { files: [file] },
      } as unknown as Event);
      await Promise.resolve();
    });

    expect(container.textContent).toContain('brief.md');

    await act(async () => {
      getSendButton(container).click();
      await Promise.resolve();
    });

    expect(onSubmit).toHaveBeenCalledTimes(1);
    expect((onSubmit.mock.calls[0]?.[0] as { files?: File[] }).files).toEqual([
      file,
    ]);

    act(() => {
      root?.unmount();
    });
    container.remove();
  });

  it('supports keyboard navigation in the NewX AI @ resource menu', async () => {
    const container = document.createElement('div');
    document.body.appendChild(container);
    let root: Root | undefined;

    const keyDown = (element: Element, key: string) => {
      element.dispatchEvent(
        new window.KeyboardEvent('keydown', {
          bubbles: true,
          cancelable: true,
          key,
        }),
      );
    };

    act(() => {
      root = createRoot(container);
      root.render(<WorkbenchPage />);
    });

    const textarea = container.querySelector(
      'textarea[aria-label="任务描述"]',
    ) as HTMLTextAreaElement;
    await act(async () => {
      Simulate.change(textarea, { target: { value: '@' } });
      await Promise.resolve();
    });

    const resourceSearch = container.querySelector(
      'input[aria-label="@资源搜索"]',
    ) as HTMLInputElement;
    const resourceButtons = () =>
      Array.from(
        container.querySelectorAll('.chat-workbench-at-menu-list button'),
      ) as HTMLButtonElement[];

    expect(resourceButtons()[0]?.dataset.active).toBe('true');
    expect(resourceButtons()[0]?.textContent).toContain('技能');

    await act(async () => {
      keyDown(resourceSearch, 'ArrowDown');
      await Promise.resolve();
    });

    expect(resourceButtons()[1]?.dataset.active).toBe('true');
    expect(resourceButtons()[1]?.textContent).toContain('知识库');

    await act(async () => {
      keyDown(resourceSearch, 'ArrowUp');
      await Promise.resolve();
    });

    expect(resourceButtons()[0]?.dataset.active).toBe('true');
    expect(resourceButtons()[0]?.textContent).toContain('技能');

    await act(async () => {
      keyDown(resourceSearch, 'ArrowDown');
      await Promise.resolve();
    });

    expect(resourceButtons()[1]?.dataset.active).toBe('true');
    expect(resourceButtons()[1]?.textContent).toContain('知识库');

    await act(async () => {
      keyDown(resourceSearch, 'Enter');
      await Promise.resolve();
    });

    expect(container.textContent).toContain('@知识库：');
    expect(
      container.querySelector('input[aria-label="@知识库搜索"]'),
    ).toBeTruthy();

    await act(async () => {
      keyDown(
        container.querySelector(
          'input[aria-label="@知识库搜索"]',
        ) as HTMLInputElement,
        'Escape',
      );
      await Promise.resolve();
    });

    expect(container.querySelector('.chat-workbench-at-inline')).toBeNull();
    expect(container.querySelector('.chat-workbench-at-menu')).toBeNull();
    expect(document.activeElement).toBe(textarea);

    act(() => {
      root?.unmount();
    });
    container.remove();
  });

  it('keeps long skill titles readable without crowding extension controls', async () => {
    const longSkillName =
      'super-long-custom-skill-title-for-cross-team-acceptance-meeting-note-generation';
    mockListSkills.mockResolvedValueOnce({
      data: {
        skills: [
          {
            id: 'skill-long-title',
            space_id: 'space-1',
            name: longSkillName,
            description: 'Long skill title layout regression fixture',
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

    const container = document.createElement('div');
    document.body.appendChild(container);
    let root: Root | undefined;

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
      await Promise.resolve();
    });

    const skillName = container.querySelector(
      '.chat-workbench-extension-name',
    ) as HTMLElement | null;
    const skillItem = container.querySelector(
      '.chat-workbench-extension-item',
    ) as HTMLElement | null;

    expect(skillName?.textContent).toBe(longSkillName);
    expect(skillName?.getAttribute('title')).toBe(longSkillName);
    expect(skillItem?.textContent).toContain('Deer');
    expect(
      skillItem
        ?.querySelector('.chat-workbench-extension-check')
        ?.getAttribute('data-selected'),
    ).toBe('true');
    expect(
      skillItem?.querySelector('.chat-workbench-extension-check span'),
    ).not.toBeNull();

    act(() => {
      root?.unmount();
    });
    container.remove();
  });

  it('shows DeerFlow slash skill suggestions and inserts the selected skill prefix', async () => {
    const container = document.createElement('div');
    document.body.appendChild(container);
    let root: Root | undefined;

    mockListSkills.mockResolvedValueOnce({
      data: {
        skills: [
          {
            id: 'skill-weekly-research',
            space_id: 'space-1',
            name: 'weekly-research',
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

    await act(async () => {
      root = createRoot(container);
      root.render(<WorkbenchPage />);
      await Promise.resolve();
    });

    const textarea = container.querySelector(
      'textarea[aria-label="任务描述"]',
    ) as HTMLTextAreaElement;
    await act(async () => {
      Simulate.focus(textarea);
      Simulate.change(textarea, {
        target: { value: '/wee' },
      } as unknown as Event);
      await Promise.resolve();
      await Promise.resolve();
    });

    const suggestions = container.querySelector(
      '[aria-label="Skill suggestions"]',
    );
    expect(suggestions).toBeTruthy();
    expect(suggestions?.getAttribute('data-placement')).toBe('bottom');
    expect(suggestions?.textContent).toContain('/weekly-research');
    expect(suggestions?.textContent).toContain('Research from trusted sources');
    expect(mockListSkills).toHaveBeenCalledWith({
      space_id: 'space-1',
      enabled: true,
    });

    const option = suggestions?.querySelector(
      'button[role="option"]',
    ) as HTMLButtonElement;
    await act(async () => {
      Simulate.click(option);
      await Promise.resolve();
    });

    expect(textarea.value).toBe('/weekly-research ');

    act(() => {
      Simulate.change(textarea, {
        target: {
          value: '/weekly-research collect market changes',
        },
      } as unknown as Event);
    });

    mockCreateTaskThread.mockResolvedValue(
      buildCreateTaskThreadResponse(
        'thread-with-slash-skill',
        '/weekly-research collect market changes',
      ),
    );

    const sendButton = getSendButton(container);
    await act(async () => {
      sendButton.click();
      await Promise.resolve();
    });

    expect(mockCreateTaskThread).toHaveBeenCalledWith({
      space_id: 'space-1',
      message: '/weekly-research collect market changes',
      config: expect.any(String),
    });
    const slashRunConfig = JSON.parse(
      mockCreateTaskThread.mock.calls[0]?.[0].config,
    );
    expect(slashRunConfig).toMatchObject({
      skills: {
        enabled: true,
        allowed_skills: [],
      },
    });
    expect(
      Object.prototype.hasOwnProperty.call(slashRunConfig, 'enable_skills'),
    ).toBe(false);

    act(() => {
      root?.unmount();
    });
    container.remove();
  });

  it('serializes only the automatic execution policy', () => {
    const resourceSelection = createDefaultWorkbenchResourceSelection();
    const runtimeSettings =
      createDefaultWorkbenchRuntimeSettings(resourceSelection);

    const runConfig = createWorkbenchRunConfig({
      message: '验证自动执行策略',
      runtimeSettings,
      ...resourceSelection,
    } as WorkbenchComposerSubmitPayload);

    expect(runConfig).toMatchObject({ requested_policy: 'auto' });
    expect(runConfig).not.toHaveProperty('mode');
    expect(runConfig).not.toHaveProperty('thinking_enabled');
    expect(runConfig).not.toHaveProperty('is_plan_mode');
    expect(runConfig).not.toHaveProperty('subagent_enabled');
  });

  it('serializes reasoning effort only when runtime reasoning is explicitly enabled', () => {
    const resourceSelection = createDefaultWorkbenchResourceSelection();
    const runtimeSettings =
      createDefaultWorkbenchRuntimeSettings(resourceSelection);
    runtimeSettings.reasoning.enabled = true;
    runtimeSettings.reasoning.effort = 'high';

    expect(
      createWorkbenchRunConfig({
        message: '验证显式推理强度',
        runtimeSettings,
        ...resourceSelection,
      } as WorkbenchComposerSubmitPayload),
    ).toMatchObject({
      requested_policy: 'auto',
      reasoning_effort: 'high',
    });
  });

  it('submits the automatic policy without a mode selector', async () => {
    const container = document.createElement('div');
    document.body.appendChild(container);
    let root: Root | undefined;

    mockCreateTaskThread.mockResolvedValue(
      buildCreateTaskThreadResponse('thread-auto-policy', '验证自动策略'),
    );

    await act(async () => {
      root = createRoot(container);
      root.render(<WorkbenchPage />);
      await Promise.resolve();
    });

    expect(
      container.querySelector('.chat-workbench-deerflow-mode-trigger'),
    ).toBeNull();

    const textarea = container.querySelector(
      'textarea[aria-label="任务描述"]',
    ) as HTMLTextAreaElement;
    act(() => {
      Simulate.change(textarea, {
        target: { value: '验证自动策略' },
      } as unknown as Event);
    });

    const sendButton = getSendButton(container);
    await act(async () => {
      sendButton.click();
      await Promise.resolve();
    });

    const runConfig = JSON.parse(
      mockCreateTaskThread.mock.calls[0]?.[0].config,
    );
    expect(runConfig).toMatchObject({ requested_policy: 'auto' });
    expect(runConfig).not.toHaveProperty('mode');
    expect(runConfig).not.toHaveProperty('thinking_enabled');
    expect(runConfig).not.toHaveProperty('is_plan_mode');
    expect(runConfig).not.toHaveProperty('subagent_enabled');
    expect(
      Object.prototype.hasOwnProperty.call(runConfig, 'reasoning_effort'),
    ).toBe(false);

    act(() => {
      root?.unmount();
    });
    container.remove();
  });

  it('navigates to the task execution page after send returns a task', async () => {
    const container = document.createElement('div');
    document.body.appendChild(container);
    let root: Root | undefined;

    mockCreateTaskThread.mockResolvedValue(
      buildCreateTaskThreadResponse('thread-1', '生成周报'),
    );

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

    const sendButton = getSendButton(container);

    await act(async () => {
      sendButton.click();
      await Promise.resolve();
    });

    expect(mockCreateTaskThread).toHaveBeenCalledWith({
      space_id: 'space-1',
      message: '帮我生成周报',
      config: expect.any(String),
    });
    const defaultRunConfig = JSON.parse(
      mockCreateTaskThread.mock.calls[0]?.[0].config,
    );
    expect(defaultRunConfig).toMatchObject({
      runtime: 'eino_adk',
      requested_policy: 'auto',
      memory_retrieval: {
        limit: 5,
        candidate_limit: 20,
        scopes: ['thread', 'long_term'],
        min_confidence: 0.2,
      },
      web_tools: {
        enabled: true,
        search: {
          enabled: true,
          max_results: 5,
        },
      },
      token_usage: {
        enabled: true,
      },
      skills: {
        enabled: true,
        allowed_skills: [],
      },
    });
    expect(
      Object.prototype.hasOwnProperty.call(
        defaultRunConfig,
        'reasoning_effort',
      ),
    ).toBe(false);
    expect(
      Object.prototype.hasOwnProperty.call(defaultRunConfig, 'enable_skills'),
    ).toBe(false);
    expect(mockNavigate).toHaveBeenCalledWith('/space/space-1/tasks/thread-1');

    act(() => {
      root?.unmount();
    });
    container.remove();
  });

  it('uploads selected files before starting a new canonical task run', async () => {
    const container = document.createElement('div');
    document.body.appendChild(container);
    let root: Root | undefined;
    const file = new File(['# Brief'], 'brief.md', {
      type: 'text/markdown',
    });

    mockCreateTaskThread.mockResolvedValue({
      data: {
        thread: {
          thread_id: 'thread-upload-1',
          space_id: 'space-1',
          creator_id: 'user-1',
          title: '请总结附件',
          status: 'idle',
          source: 'web',
          progress: 0,
          last_user_message: '',
          last_agent_message: '',
          created_at: 1717000000,
          updated_at: 1717000000,
        },
      },
      code: 0,
      msg: '',
    });
    mockUploadTaskThreadFiles.mockResolvedValue({
      data: {
        files: [
          {
            file_id: 'file-1',
            file_name: 'brief.md',
            virtual_path: '/mnt/user-data/uploads/brief.md',
            content_type: 'text/markdown',
            size_bytes: 7,
            created_at: 1717000000,
          },
        ],
      },
      code: 0,
      msg: '',
    });
    mockCreateTaskThreadRun.mockResolvedValue({
      data: {
        run_id: 'run-upload-1',
        thread_id: 'thread-upload-1',
        status: 'queued',
      },
      message: {
        message_id: 'msg-upload-1',
        thread_id: 'thread-upload-1',
        run_id: 'run-upload-1',
        role: 'user',
        content: '请总结附件',
        metadata: '{}',
        created_at: 1717000000,
      },
      code: 0,
      msg: '',
    });

    act(() => {
      root = createRoot(container);
      root.render(<WorkbenchPage />);
    });

    const fileInput = container.querySelector(
      'input[type="file"]',
    ) as HTMLInputElement;
    await act(async () => {
      Simulate.change(fileInput, {
        target: { files: [file] },
      } as unknown as Event);
      await Promise.resolve();
    });
    const textarea = container.querySelector(
      'textarea[aria-label="任务描述"]',
    ) as HTMLTextAreaElement;
    act(() => {
      Simulate.change(textarea, {
        target: { value: '请总结附件' },
      } as unknown as Event);
    });

    await act(async () => {
      getSendButton(container).click();
      await Promise.resolve();
      await Promise.resolve();
      await Promise.resolve();
    });

    expect(mockCreateTaskThread).toHaveBeenCalledWith({
      space_id: 'space-1',
      message: '请总结附件',
      config: expect.any(String),
      defer_start: true,
    });
    expect(mockUploadTaskThreadFiles).toHaveBeenCalledWith({
      thread_id: 'thread-upload-1',
      files: [file],
      space_id: 'space-1',
    });
    expect(mockCreateTaskThreadRun).toHaveBeenCalledWith({
      thread_id: 'thread-upload-1',
      space_id: 'space-1',
      input: expect.any(String),
      config: expect.any(String),
      metadata: expect.any(String),
      idempotency_key: expect.any(String),
      message_content: '请总结附件',
      message_metadata: expect.any(String),
    });
    expect(
      JSON.parse(mockCreateTaskThreadRun.mock.calls[0]?.[0].input),
    ).toMatchObject({
      messages: [
        {
          role: 'user',
          content: '请总结附件',
        },
      ],
      uploaded_files: [
        {
          file_name: 'brief.md',
          virtual_path: '/mnt/user-data/uploads/brief.md',
          content_type: 'text/markdown',
          size_bytes: 7,
        },
      ],
    });
    expect(mockNavigate).toHaveBeenCalledWith(
      '/space/space-1/tasks/thread-upload-1',
    );

    act(() => {
      root?.unmount();
    });
    container.remove();
  });

  it('submits the task when pressing Enter in the composer input', async () => {
    const container = document.createElement('div');
    document.body.appendChild(container);
    let root: Root | undefined;

    mockCreateTaskThread.mockResolvedValue(
      buildCreateTaskThreadResponse('thread-enter-submit', '回车发送任务'),
    );

    act(() => {
      root = createRoot(container);
      root.render(<WorkbenchPage />);
    });

    const textarea = container.querySelector(
      'textarea[aria-label="任务描述"]',
    ) as HTMLTextAreaElement;
    act(() => {
      Simulate.change(textarea, {
        target: { value: '回车发送任务' },
      } as unknown as Event);
    });

    await act(async () => {
      Simulate.keyDown(textarea, {
        key: 'Enter',
      } as unknown as KeyboardEvent<HTMLTextAreaElement>);
      await Promise.resolve();
    });

    expect(mockCreateTaskThread).toHaveBeenCalledWith({
      space_id: 'space-1',
      message: '回车发送任务',
      config: expect.any(String),
    });
    expect(mockNavigate).toHaveBeenCalledWith(
      '/space/space-1/tasks/thread-enter-submit',
    );

    act(() => {
      root?.unmount();
    });
    container.remove();
  });

  it('does not submit the task when pressing Shift Enter in the composer input', () => {
    const container = document.createElement('div');
    document.body.appendChild(container);
    let root: Root | undefined;

    act(() => {
      root = createRoot(container);
      root.render(<WorkbenchPage />);
    });

    const textarea = container.querySelector(
      'textarea[aria-label="任务描述"]',
    ) as HTMLTextAreaElement;
    act(() => {
      Simulate.change(textarea, {
        target: { value: '保留多行输入' },
      } as unknown as Event);
      Simulate.keyDown(textarea, {
        key: 'Enter',
        shiftKey: true,
      } as unknown as KeyboardEvent<HTMLTextAreaElement>);
    });

    expect(mockCreateTaskThread).not.toHaveBeenCalled();

    act(() => {
      root?.unmount();
    });
    container.remove();
  });

  it('announces the newly created task thread before navigating to detail', async () => {
    const dispatchSpy = vi.spyOn(window, 'dispatchEvent');
    const container = document.createElement('div');
    document.body.appendChild(container);
    let root: Root | undefined;

    mockCreateTaskThread.mockResolvedValue(
      buildCreateTaskThreadResponse('thread-created-now', '即时显示任务'),
    );

    act(() => {
      root = createRoot(container);
      root.render(<WorkbenchPage />);
    });

    const textarea = container.querySelector(
      'textarea[aria-label="任务描述"]',
    ) as HTMLTextAreaElement;
    act(() => {
      Simulate.change(textarea, {
        target: { value: '即时显示任务' },
      } as unknown as Event);
    });

    const sendButton = getSendButton(container);

    await act(async () => {
      sendButton.click();
      await Promise.resolve();
    });

    const upsertEvent = dispatchSpy.mock.calls
      .map(([event]) => event)
      .find(event => event.type === 'coze:workspace-task-thread-upsert') as
      | CustomEvent
      | undefined;

    expect(upsertEvent?.detail).toMatchObject({
      space_id: 'space-1',
      thread: {
        thread_id: 'thread-created-now',
        title: '即时显示任务',
      },
    });
    expect(mockNavigate).toHaveBeenCalledWith(
      '/space/space-1/tasks/thread-created-now',
    );

    dispatchSpy.mockRestore();
    act(() => {
      root?.unmount();
    });
    container.remove();
  });

  it('navigates to the task list when thread creation returns no thread', async () => {
    const container = document.createElement('div');
    document.body.appendChild(container);
    let root: Root | undefined;

    mockCreateTaskThread.mockResolvedValue({
      data: {},
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

    const sendButton = getSendButton(container);

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

  it('shows the thread creation error without navigating', async () => {
    const container = document.createElement('div');
    document.body.appendChild(container);
    let root: Root | undefined;

    mockCreateTaskThread.mockRejectedValue(new Error('chat failed'));

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

    const sendButton = getSendButton(container);

    await act(async () => {
      sendButton.click();
      await Promise.resolve();
    });

    expect(mockNavigate).not.toHaveBeenCalled();
    expect(container.textContent).toContain('chat failed');

    act(() => {
      root?.unmount();
    });
    container.remove();
  });

  it('can disable default skill usage from the extension menu', async () => {
    const container = document.createElement('div');
    document.body.appendChild(container);
    let root: Root | undefined;

    mockCreateTaskThread.mockResolvedValue(
      buildCreateTaskThreadResponse(
        'thread-with-disabled-skill',
        '关闭默认技能',
      ),
    );

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
        target: { value: '关闭默认技能' },
      } as unknown as Event);
    });

    const sendButton = getSendButton(container);

    await act(async () => {
      sendButton.click();
      await Promise.resolve();
    });

    expect(mockCreateTaskThread).toHaveBeenCalledWith({
      space_id: 'space-1',
      message: '关闭默认技能',
      config: expect.any(String),
    });
    expect(
      JSON.parse(mockCreateTaskThread.mock.calls[0]?.[0].config),
    ).toMatchObject({
      requested_policy: 'auto',
      model_type: 100002,
      model_name: 'deepseek-v4-pro',
      enable_skills: [],
      skills: {
        enabled: false,
        allowed_skills: [],
      },
    });
    expect(
      Object.prototype.hasOwnProperty.call(
        JSON.parse(mockCreateTaskThread.mock.calls[0]?.[0].config),
        'reasoning_effort',
      ),
    ).toBe(false);

    act(() => {
      root?.unmount();
    });
    container.remove();
  });

  it('can exclude default MCP tools while keeping MCP auto usage enabled', async () => {
    const container = document.createElement('div');
    document.body.appendChild(container);
    let root: Root | undefined;

    mockCreateTaskThread.mockResolvedValue(
      buildCreateTaskThreadResponse(
        'thread-with-mcp-selection',
        '用 GitHub MCP 搜索 Coze 仓库',
      ),
    );

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
      await Promise.resolve();
    });

    const mcpTabButton = Array.from(container.querySelectorAll('button')).find(
      button => button.textContent?.includes('MCP'),
    ) as HTMLButtonElement;
    await act(async () => {
      mcpTabButton.click();
      await Promise.resolve();
    });

    expect(mockListMCPToolRegistryEntries).toHaveBeenCalledWith({
      space_id: 'space-1',
    });
    expect(container.textContent).toContain('github');
    expect(container.textContent).not.toContain('search_repositories');

    const mcpToolButton = Array.from(container.querySelectorAll('button')).find(
      button => button.textContent?.includes('github'),
    ) as HTMLButtonElement;
    act(() => {
      mcpToolButton.click();
    });
    expect(container.textContent).toContain('已启用 2/3');

    const textarea = container.querySelector(
      'textarea[aria-label="任务描述"]',
    ) as HTMLTextAreaElement;
    act(() => {
      Simulate.change(textarea, {
        target: { value: '用 GitHub MCP 搜索 Coze 仓库' },
      } as unknown as Event);
    });

    const sendButton = getSendButton(container);
    await act(async () => {
      sendButton.click();
      await Promise.resolve();
    });

    expect(mockCreateTaskThread).toHaveBeenCalledWith({
      space_id: 'space-1',
      message: '用 GitHub MCP 搜索 Coze 仓库',
      config: expect.any(String),
    });

    expect(
      JSON.parse(mockCreateTaskThread.mock.calls[0]?.[0].config),
    ).toMatchObject({
      enable_mcp: ['mcp_7656806170694254592_query'],
      mcp_tools: {
        enabled: true,
        allowed_tools: ['mcp_7656806170694254592_query'],
      },
    });

    act(() => {
      root?.unmount();
    });
    container.remove();
  });

  it('persists extension usage switches across composer remounts', async () => {
    const container = document.createElement('div');
    document.body.appendChild(container);
    let root: Root | undefined;

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
      await Promise.resolve();
    });

    const mcpTabButton = Array.from(container.querySelectorAll('button')).find(
      button => button.textContent?.includes('MCP'),
    ) as HTMLButtonElement;
    await act(async () => {
      mcpTabButton.click();
      await Promise.resolve();
    });

    const mcpToolButton = Array.from(container.querySelectorAll('button')).find(
      button => button.textContent?.includes('github'),
    ) as HTMLButtonElement;
    await act(async () => {
      mcpToolButton.click();
      await Promise.resolve();
    });

    act(() => {
      root?.unmount();
    });

    mockCreateTaskThread.mockResolvedValue(
      buildCreateTaskThreadResponse(
        'thread-with-persisted-extension-usage',
        '复用刷新前的拓展开关',
      ),
    );

    await act(async () => {
      root = createRoot(container);
      root.render(<WorkbenchPage />);
      await Promise.resolve();
    });

    const textarea = container.querySelector(
      'textarea[aria-label="任务描述"]',
    ) as HTMLTextAreaElement;
    act(() => {
      Simulate.change(textarea, {
        target: { value: '复用刷新前的拓展开关' },
      } as unknown as Event);
    });

    const sendButton = getSendButton(container);
    await act(async () => {
      sendButton.click();
      await Promise.resolve();
    });

    const runtimeSettings = JSON.parse(
      mockCreateTaskThread.mock.calls[0]?.[0].config,
    );
    expect(runtimeSettings).toMatchObject({
      mcp_tools: {
        enabled: true,
        allowed_tools: ['mcp_7656806170694254592_query'],
      },
    });

    act(() => {
      root?.unmount();
    });
    container.remove();
  });

  it('enables MCP auto-discovery without manual tool selection', async () => {
    const container = document.createElement('div');
    document.body.appendChild(container);
    let root: Root | undefined;

    mockCreateTaskThread.mockResolvedValue(
      buildCreateTaskThreadResponse(
        'thread-with-auto-mcp',
        '用 GitHub MCP 搜索 Coze 仓库',
      ),
    );

    await act(async () => {
      root = createRoot(container);
      root.render(<WorkbenchPage />);
      await Promise.resolve();
    });

    const textarea = container.querySelector(
      'textarea[aria-label="任务描述"]',
    ) as HTMLTextAreaElement;
    act(() => {
      Simulate.change(textarea, {
        target: { value: '用 GitHub MCP 搜索 Coze 仓库' },
      } as unknown as Event);
    });

    const sendButton = getSendButton(container);
    await act(async () => {
      sendButton.click();
      await Promise.resolve();
    });

    const runtimeSettings = JSON.parse(
      mockCreateTaskThread.mock.calls[0]?.[0].config,
    );
    expect(runtimeSettings).toMatchObject({
      enable_mcp: [],
      mcp_tools: {
        enabled: true,
      },
    });
    expect(
      Object.prototype.hasOwnProperty.call(
        runtimeSettings.mcp_tools,
        'allowed_tools',
      ),
    ).toBe(false);

    act(() => {
      root?.unmount();
    });
    container.remove();
  });

  it('passes skills selected from the at menu in the chat request', async () => {
    const container = document.createElement('div');
    document.body.appendChild(container);
    let root: Root | undefined;

    mockCreateTaskThread.mockResolvedValue(
      buildCreateTaskThreadResponse(
        'thread-with-at-skill-selection',
        '用选择的技能创建总结能力',
      ),
    );

    act(() => {
      root = createRoot(container);
      root.render(<WorkbenchPage />);
    });

    const textarea = container.querySelector(
      'textarea[aria-label="任务描述"]',
    ) as HTMLTextAreaElement;
    act(() => {
      Simulate.change(textarea, {
        target: { value: '用选择的技能创建总结能力' },
      } as unknown as Event);
    });

    const atButton = container.querySelector(
      'button[aria-label="添加上下文"]',
    ) as HTMLButtonElement;
    await act(async () => {
      atButton.click();
      await Promise.resolve();
    });

    const skillTypeButton = Array.from(
      container.querySelectorAll('.chat-workbench-at-menu-list button'),
    ).find(button => button.textContent?.includes('技能')) as
      | HTMLButtonElement
      | undefined;
    expect(skillTypeButton).toBeTruthy();

    await act(async () => {
      skillTypeButton?.click();
      await Promise.resolve();
    });

    const skillButton = Array.from(container.querySelectorAll('button')).find(
      button => button.textContent?.includes('Research Skill'),
    ) as HTMLButtonElement;
    expect(skillButton).toBeTruthy();

    act(() => {
      skillButton.click();
    });

    const sendButton = getSendButton(container);
    await act(async () => {
      sendButton.click();
      await Promise.resolve();
    });

    expect(mockCreateTaskThread).toHaveBeenCalledWith({
      space_id: 'space-1',
      message: '用选择的技能创建总结能力 @Research Skill',
      config: expect.any(String),
    });
    expect(
      JSON.parse(mockCreateTaskThread.mock.calls[0]?.[0].config),
    ).toMatchObject({
      enable_skills: ['skill-101'],
      skills: {
        enabled: true,
        allowed_skills: ['skill-101'],
      },
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

    mockCreateTaskThread.mockResolvedValue(
      buildCreateTaskThreadResponse(
        'thread-with-model-selection',
        '指定模型回答',
      ),
    );

    await act(async () => {
      root = createRoot(container);
      root.render(<WorkbenchPage />);
      await Promise.resolve();
    });

    expect(mockGetTypeList).toHaveBeenCalledWith('space-1');
    expect(container.textContent).toContain('DeepSeek V4 Pro (Thinking)');
    expect(container.textContent).not.toContain('Pro拓展deepseek-v4-pro');

    const selectorButton = container.querySelector(
      'button[aria-label="选择模型"]',
    ) as HTMLButtonElement;
    act(() => {
      selectorButton.click();
    });

    const modelButton = Array.from(container.querySelectorAll('button')).find(
      button => button.textContent?.includes('GPT 4.1'),
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

    const sendButton = getSendButton(container);

    await act(async () => {
      sendButton.click();
      await Promise.resolve();
    });

    expect(mockCreateTaskThread).toHaveBeenCalledWith({
      space_id: 'space-1',
      message: '指定模型回答',
      config: expect.any(String),
    });
    expect(
      JSON.parse(mockCreateTaskThread.mock.calls[0]?.[0].config),
    ).toMatchObject({
      requested_policy: 'auto',
      model_type: 100003,
      model_name: 'gpt-4.1',
    });
    expect(
      Object.prototype.hasOwnProperty.call(
        JSON.parse(mockCreateTaskThread.mock.calls[0]?.[0].config),
        'reasoning_effort',
      ),
    ).toBe(false);

    act(() => {
      root?.unmount();
    });
    container.remove();
  });

  it('hides runtime settings in the DeerFlow home composer and sends safe defaults', async () => {
    const container = document.createElement('div');
    document.body.appendChild(container);
    let root: Root | undefined;

    mockCreateTaskThread.mockResolvedValue(
      buildCreateTaskThreadResponse('thread-with-runtime-defaults', '稳定执行'),
    );

    await act(async () => {
      root = createRoot(container);
      root.render(<WorkbenchPage />);
      await Promise.resolve();
    });

    expect(container.querySelector('button[aria-label="运行设置"]')).toBeNull();

    const textarea = container.querySelector(
      'textarea[aria-label="任务描述"]',
    ) as HTMLTextAreaElement;
    act(() => {
      Simulate.change(textarea, {
        target: { value: '稳定执行' },
      } as unknown as Event);
    });

    const sendButton = getSendButton(container);

    await act(async () => {
      sendButton.click();
      await Promise.resolve();
    });

    const runtimeSettings = JSON.parse(
      mockCreateTaskThread.mock.calls[0]?.[0].config,
    );
    expect(runtimeSettings).toMatchObject({
      web_tools: {
        enabled: true,
        http: {
          enabled: false,
          allowed_hosts: [],
        },
        search: {
          enabled: true,
          max_results: 5,
        },
      },
      token_usage: {
        enabled: true,
      },
    });
    expect(runtimeSettings.model_retry).toBeUndefined();
    expect(runtimeSettings.model_failover).toBeUndefined();

    act(() => {
      root?.unmount();
    });
    container.remove();
  });

  it('sends DeerFlow extension defaults from the composer', async () => {
    const container = document.createElement('div');
    document.body.appendChild(container);
    let root: Root | undefined;

    mockCreateTaskThread.mockResolvedValue(
      buildCreateTaskThreadResponse(
        'thread-with-runtime-aggregation',
        '聚合运行设置',
      ),
    );

    await act(async () => {
      root = createRoot(container);
      root.render(<WorkbenchPage />);
      await Promise.resolve();
    });

    const extensionButton = Array.from(
      container.querySelectorAll('button'),
    ).find(button => button.textContent?.includes('拓展')) as HTMLButtonElement;
    await act(async () => {
      extensionButton.click();
      await Promise.resolve();
      await Promise.resolve();
    });

    expect(container.querySelector('button[aria-label="运行设置"]')).toBeNull();

    const textarea = container.querySelector(
      'textarea[aria-label="任务描述"]',
    ) as HTMLTextAreaElement;
    act(() => {
      Simulate.change(textarea, {
        target: { value: '聚合运行设置' },
      } as unknown as Event);
    });

    const sendButton = getSendButton(container);

    await act(async () => {
      sendButton.click();
      await Promise.resolve();
    });

    expect(mockCreateTaskThread).toHaveBeenCalledWith({
      space_id: 'space-1',
      message: '聚合运行设置',
      config: expect.any(String),
    });

    const runtimeSettings = JSON.parse(
      mockCreateTaskThread.mock.calls[0]?.[0].config,
    );
    expect(runtimeSettings).toMatchObject({
      requested_policy: 'auto',
      skills: {
        enabled: true,
        allowed_skills: [],
      },
      mcp_tools: {
        enabled: true,
      },
    });
    expect(
      Object.prototype.hasOwnProperty.call(
        runtimeSettings.mcp_tools,
        'allowed_tools',
      ),
    ).toBe(false);
    expect(
      Object.prototype.hasOwnProperty.call(runtimeSettings, 'reasoning_effort'),
    ).toBe(false);

    act(() => {
      root?.unmount();
    });
    container.remove();
  });

  it('keeps web fetch disabled from the DeerFlow home composer', async () => {
    const container = document.createElement('div');
    document.body.appendChild(container);
    let root: Root | undefined;

    mockCreateTaskThread.mockResolvedValue(
      buildCreateTaskThreadResponse('thread-with-web-fetch', '读取网页资料'),
    );

    await act(async () => {
      root = createRoot(container);
      root.render(<WorkbenchPage />);
      await Promise.resolve();
    });

    expect(container.querySelector('button[aria-label="运行设置"]')).toBeNull();
    expect(container.querySelector('button[aria-label="网页读取"]')).toBeNull();
    expect(
      container.querySelector('input[aria-label="网页读取允许域名"]'),
    ).toBeNull();

    const textarea = container.querySelector(
      'textarea[aria-label="任务描述"]',
    ) as HTMLTextAreaElement;
    act(() => {
      Simulate.change(textarea, {
        target: { value: '读取网页资料' },
      } as unknown as Event);
    });

    const sendButton = getSendButton(container);

    await act(async () => {
      sendButton.click();
      await Promise.resolve();
    });

    const runtimeSettings = JSON.parse(
      mockCreateTaskThread.mock.calls[0]?.[0].config,
    );
    expect(runtimeSettings).toMatchObject({
      web_tools: {
        enabled: true,
        http: {
          enabled: false,
          allowed_hosts: [],
          timeout_ms: 10000,
          max_response_bytes: 262144,
        },
        search: {
          enabled: true,
          max_results: 5,
        },
      },
    });

    act(() => {
      root?.unmount();
    });
    container.remove();
  });
});
