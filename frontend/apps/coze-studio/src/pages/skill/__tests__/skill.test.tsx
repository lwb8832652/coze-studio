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
import { act } from 'react-dom/test-utils';
import { createRoot, type Root } from 'react-dom/client';
import { workbenchSkill } from '@coze-studio/api-schema';

globalThis.IS_REACT_ACT_ENVIRONMENT = true;

const mockUseParams = vi.hoisted(() => vi.fn(() => ({ space_id: 'space-1' })));
const mockNavigate = vi.hoisted(() => vi.fn());
const mockListSkills = vi.hoisted(() => vi.fn());
const mockDeleteSkill = vi.hoisted(() => vi.fn());
const mockTestRunSkill = vi.hoisted(() => vi.fn());
const mockUpdateSkill = vi.hoisted(() => vi.fn());
const mockListSkillToolCandidates = vi.hoisted(() => vi.fn());
const mockListSkillVersions = vi.hoisted(() => vi.fn());
const mockListSkillVersionResources = vi.hoisted(() => vi.fn());

vi.mock('react-router-dom', () => ({
  useNavigate: () => mockNavigate,
  useParams: mockUseParams,
}));

/* eslint-disable @typescript-eslint/naming-convention -- Semi mock exports keep component names and aria props. */
vi.mock('@coze-arch/coze-design', () => ({
  Banner: ({ description }: { description?: ReactNode }) => (
    <div>{description}</div>
  ),
  Button: ({
    'aria-label': ariaLabel,
    children,
    disabled,
    onClick,
  }: {
    'aria-label'?: string;
    children?: ReactNode;
    disabled?: boolean;
    onClick?: () => void;
  }) => (
    <button
      type="button"
      aria-label={ariaLabel}
      disabled={disabled}
      onClick={onClick}
    >
      {children}
    </button>
  ),
  ButtonGroup: ({ children }: { children?: ReactNode }) => (
    <div>{children}</div>
  ),
  Checkbox: ({
    'aria-label': ariaLabel,
    checked,
    children,
    disabled,
    onChange,
  }: {
    'aria-label'?: string;
    checked?: boolean;
    children?: ReactNode;
    disabled?: boolean;
    onChange?: (event: { target: { checked: boolean } }) => void;
  }) => (
    <label>
      <input
        aria-label={ariaLabel}
        checked={checked}
        disabled={disabled}
        type="checkbox"
        onChange={event =>
          onChange?.({ target: { checked: event.target.checked } })
        }
      />
      {children}
    </label>
  ),
  IconButton: ({
    'aria-label': ariaLabel,
    disabled,
    icon,
    onClick,
  }: {
    'aria-label'?: string;
    disabled?: boolean;
    icon?: ReactNode;
    onClick?: () => void;
  }) => (
    <button
      type="button"
      aria-label={ariaLabel}
      disabled={disabled}
      onClick={onClick}
    >
      {icon}
    </button>
  ),
  Input: ({
    'aria-label': ariaLabel,
    onChange,
    placeholder,
    value,
  }: {
    'aria-label'?: string;
    onChange?: (value: string) => void;
    placeholder?: string;
    value?: string;
  }) => (
    <input
      aria-label={ariaLabel}
      placeholder={placeholder}
      value={value}
      onChange={event => onChange?.(event.target.value)}
    />
  ),
  SideSheet: ({
    children,
    title,
    visible,
  }: {
    children?: ReactNode;
    title?: ReactNode;
    visible?: boolean;
  }) =>
    visible ? (
      <aside>
        {title}
        {children}
      </aside>
    ) : null,
  Spin: () => <span>加载中...</span>,
  Switch: ({
    'aria-label': ariaLabel,
    checked,
    disabled,
    loading,
    onChange,
  }: {
    'aria-label'?: string;
    checked?: boolean;
    disabled?: boolean;
    loading?: boolean;
    onChange?: (checked: boolean) => void;
  }) => (
    <button
      type="button"
      aria-label={ariaLabel}
      aria-checked={checked}
      disabled={disabled || loading}
      role="switch"
      onClick={() => onChange?.(!checked)}
    />
  ),
  TabPane: ({ children }: { children?: ReactNode }) => <div>{children}</div>,
  Tabs: ({ children }: { children?: ReactNode }) => <div>{children}</div>,
  TextArea: ({
    'aria-label': ariaLabel,
    onChange,
    placeholder,
    value,
  }: {
    'aria-label'?: string;
    onChange?: (value: string) => void;
    placeholder?: string;
    value?: string;
  }) => (
    <textarea
      aria-label={ariaLabel}
      placeholder={placeholder}
      value={value}
      onChange={event => onChange?.(event.target.value)}
    />
  ),
  Upload: ({ children }: { children?: ReactNode }) => <div>{children}</div>,
}));

vi.mock('@coze-arch/coze-design/icons', () => ({
  IconCozBell: () => <span />,
  IconCozHistory: () => <span />,
  IconCozMore: () => <span />,
  IconCozPlus: () => <span />,
  IconCozSetting: () => <span />,
  IconCozUpload: () => <span />,
}));
/* eslint-enable @typescript-eslint/naming-convention -- Restore naming checks after component mocks. */

vi.mock('../service', () => ({
  createSkill: vi.fn(),
  deleteSkill: mockDeleteSkill,
  importSkill: vi.fn(),
  exportSkillVersion: vi.fn(),
  listSkillToolCandidates: mockListSkillToolCandidates,
  listSkillVersionResources: mockListSkillVersionResources,
  listSkillVersions: mockListSkillVersions,
  listSkills: mockListSkills,
  rollbackSkillVersion: vi.fn(),
  testRunSkill: mockTestRunSkill,
  updateSkill: mockUpdateSkill,
  updateSkillVersionContent: vi.fn(),
  updateSkillVersionResource: vi.fn(),
}));

import {
  getVisibleSkills,
  type SkillTypeFilter,
} from '../skill-page-components';
import SkillPage from '../index';

const skill = {
  id: 'skill-1',
  space_id: 'space-1',
  name: 'Ping Skill',
  description: '',
  type: workbenchSkill.SkillType.DeerSkill,
  version: '1.0.0',
  enabled: true,
  input_schema: '{}',
  output_schema: '{}',
  executor: '{}',
  permissions: '{}',
  created_at: 1717000000000,
  updated_at: 1717000300000,
};

describe('SkillPage', () => {
  beforeEach(() => {
    mockUseParams.mockReturnValue({ space_id: 'space-1' });
    mockNavigate.mockReset();
    mockDeleteSkill.mockReset();
    mockUpdateSkill.mockReset();
    mockListSkillToolCandidates.mockReset();
    mockListSkills.mockResolvedValue({
      data: { skills: [skill] },
      code: 0,
      msg: '',
    });
    mockTestRunSkill.mockReset();
    mockUpdateSkill.mockResolvedValue({
      data: { ...skill, enabled: false },
      code: 0,
      msg: '',
    });
    mockDeleteSkill.mockResolvedValue({
      data: { ...skill, enabled: false },
      code: 0,
      msg: '',
    });
    mockListSkillVersions.mockResolvedValue({
      data: { versions: [] },
      code: 0,
      msg: '',
    });
    mockListSkillVersionResources.mockResolvedValue({
      data: { resources: [] },
      code: 0,
      msg: '',
    });
    mockListSkillToolCandidates.mockResolvedValue({
      data: { tools: [] },
      code: 0,
      msg: '',
    });
  });

  it('renders the DeerFlow-style skill settings structure', async () => {
    const container = document.createElement('div');
    document.body.appendChild(container);
    let root: Root | undefined;

    await act(async () => {
      root = createRoot(container);
      root.render(<SkillPage />);
      await Promise.resolve();
    });

    expect(container.textContent).toContain('技能');
    expect(container.textContent).not.toContain('NewX AI 专属助理准备好');
    expect(container.textContent).not.toContain('去聊天专属助理');
    expect(container.textContent).toContain(
      '管理 Agent Skill 配置和启用状态。',
    );
    expect(container.textContent).toContain('公共');
    expect(container.textContent).toContain('自定义');
    expect(container.textContent).toContain('新建技能');
    expect(container.textContent).not.toContain('全部技能');
    expect(container.textContent).not.toContain('内置');
    expect(container.textContent).not.toContain('脚本');
    expect(container.textContent).not.toContain('工作流');
    expect(container.querySelector('input[aria-label="搜索技能"]')).toBeNull();
    expect(container.textContent).not.toContain('试运行');
    expect(container.textContent).not.toContain('删除');
    expect(container.querySelector('[role="switch"]')).toBeTruthy();

    act(() => {
      root?.unmount();
    });
    container.remove();
  });

  it('opens the DeerFlow skill creator task entry', async () => {
    const container = document.createElement('div');
    document.body.appendChild(container);
    let root: Root | undefined;

    await act(async () => {
      root = createRoot(container);
      root.render(<SkillPage />);
      await Promise.resolve();
    });

    const createButton = Array.from(container.querySelectorAll('button')).find(
      button => button.textContent === '新建技能',
    ) as HTMLButtonElement;

    await act(async () => {
      createButton.click();
      await Promise.resolve();
    });

    expect(mockNavigate).toHaveBeenCalledWith(
      '/space/space-1/chats/new?mode=skill',
    );

    act(() => {
      root?.unmount();
    });
    container.remove();
  });

  it('filters visible settings skills by public and custom DeerFlow categories', () => {
    const customSkill = {
      ...skill,
      id: 'skill-custom',
      name: 'Custom Planner',
      type: workbenchSkill.SkillType.CustomSkill,
    };
    const publicSkill = {
      ...skill,
      id: 'skill-public',
      name: 'Public Research',
      type: workbenchSkill.SkillType.PublicSkill,
    };
    const bootstrapSkill = {
      ...skill,
      id: 'skill-bootstrap',
      name: 'Bootstrap Research',
      type: workbenchSkill.SkillType.DeerSkill,
    };
    const deerSkill = {
      ...skill,
      id: 'skill-deer',
      name: 'DeerFlow Builtin',
      type: workbenchSkill.SkillType.DeerSkill,
    };
    const workflowSkill = {
      ...skill,
      id: 'skill-workflow',
      name: 'Workflow Skill',
      type: workbenchSkill.SkillType.Workflow,
    };
    const skills = [
      customSkill,
      publicSkill,
      bootstrapSkill,
      deerSkill,
      workflowSkill,
    ];

    expect(
      getVisibleSkills(skills, '', 'custom' as SkillTypeFilter).map(
        item => item.id,
      ),
    ).toEqual(['skill-custom']);
    expect(
      getVisibleSkills(skills, '', 'public' as SkillTypeFilter).map(
        item => item.id,
      ),
    ).toEqual(['skill-public', 'skill-bootstrap', 'skill-deer']);
  });

  it('renders DeerFlow-style loading and empty states', async () => {
    const loadingContainer = document.createElement('div');
    document.body.appendChild(loadingContainer);
    let loadingRoot: Root | undefined;
    mockListSkills.mockImplementationOnce(() => new Promise(() => undefined));

    await act(async () => {
      loadingRoot = createRoot(loadingContainer);
      loadingRoot.render(<SkillPage />);
      await Promise.resolve();
    });

    expect(loadingContainer.textContent).toContain('加载中...');
    expect(loadingContainer.querySelector('.coze-prototype-empty')).toBeNull();

    act(() => {
      loadingRoot?.unmount();
    });
    loadingContainer.remove();

    const emptyContainer = document.createElement('div');
    document.body.appendChild(emptyContainer);
    let emptyRoot: Root | undefined;
    mockListSkills.mockResolvedValueOnce({
      data: { skills: [] },
      code: 0,
      msg: '',
    });

    await act(async () => {
      emptyRoot = createRoot(emptyContainer);
      emptyRoot.render(<SkillPage />);
      await Promise.resolve();
    });

    expect(emptyContainer.textContent).toContain('还没有技能');
    expect(emptyContainer.textContent).toContain(
      '将你的 Agent Skill 文件夹放在 DeerFlow 根目录下的 `/skills/custom` 文件夹中。',
    );
    expect(emptyContainer.textContent).toContain('创建你的第一个技能');
    expect(emptyContainer.textContent).not.toContain('暂无技能');
    expect(emptyContainer.querySelector('.coze-prototype-empty')).toBeNull();

    const createFirstButton = Array.from(
      emptyContainer.querySelectorAll('button'),
    ).find(button => button.textContent === '创建你的第一个技能') as
      | HTMLButtonElement
      | undefined;
    expect(createFirstButton).toBeTruthy();

    await act(async () => {
      createFirstButton?.click();
      await Promise.resolve();
    });

    expect(mockNavigate).toHaveBeenCalledWith(
      '/space/space-1/chats/new?mode=skill',
    );

    act(() => {
      emptyRoot?.unmount();
    });
    emptyContainer.remove();
  });

  it('opens skill version management from the skill row', async () => {
    const container = document.createElement('div');
    document.body.appendChild(container);
    let root: Root | undefined;

    await act(async () => {
      root = createRoot(container);
      root.render(<SkillPage />);
      await Promise.resolve();
    });

    const moreButton = container.querySelector(
      'button[aria-label="更多 Ping Skill"]',
    ) as HTMLButtonElement;

    await act(async () => {
      moreButton.click();
      await Promise.resolve();
    });

    const manageButton = Array.from(container.querySelectorAll('button')).find(
      button => button.textContent === '版本管理',
    ) as HTMLButtonElement;

    await act(async () => {
      manageButton.click();
      await Promise.resolve();
    });

    expect(container.textContent).toContain('Ping Skill · 版本管理');
    expect(container.textContent).toContain('版本历史');
    expect(mockListSkillVersions).toHaveBeenCalledWith({
      skill_id: 'skill-1',
    });

    act(() => {
      root?.unmount();
    });
    container.remove();
  });

  it('toggles enabled state through the skill update API', async () => {
    const container = document.createElement('div');
    document.body.appendChild(container);
    let root: Root | undefined;

    mockListSkills
      .mockResolvedValueOnce({
        data: { skills: [skill] },
        code: 0,
        msg: '',
      })
      .mockResolvedValueOnce({
        data: { skills: [{ ...skill, enabled: false }] },
        code: 0,
        msg: '',
      });

    await act(async () => {
      root = createRoot(container);
      root.render(<SkillPage />);
      await Promise.resolve();
    });

    const disableButton = container.querySelector(
      'button[role="switch"][aria-label="关闭 Ping Skill"]',
    ) as HTMLButtonElement;

    await act(async () => {
      disableButton.click();
      await Promise.resolve();
    });

    expect(mockUpdateSkill).toHaveBeenCalledWith({
      id: 'skill-1',
      space_id: 'space-1',
      name: 'Ping Skill',
      description: '',
      type: workbenchSkill.SkillType.DeerSkill,
      version: '1.0.0',
      enabled: false,
      input_schema: '{}',
      output_schema: '{}',
      executor: '{}',
      permissions: '{}',
    });
    expect(
      container.querySelector(
        'button[role="switch"][aria-label="开启 Ping Skill"]',
      ),
    ).toBeTruthy();

    act(() => {
      root?.unmount();
    });
    container.remove();
  });

  it('soft deletes a skill after confirmation', async () => {
    const container = document.createElement('div');
    document.body.appendChild(container);
    let root: Root | undefined;
    const originalConfirm = window.confirm;
    window.confirm = vi.fn(() => true);

    mockListSkills
      .mockResolvedValueOnce({
        data: { skills: [skill] },
        code: 0,
        msg: '',
      })
      .mockResolvedValueOnce({
        data: { skills: [] },
        code: 0,
        msg: '',
      });

    await act(async () => {
      root = createRoot(container);
      root.render(<SkillPage />);
      await Promise.resolve();
    });

    const moreButton = container.querySelector(
      'button[aria-label="更多 Ping Skill"]',
    ) as HTMLButtonElement;

    await act(async () => {
      moreButton.click();
      await Promise.resolve();
    });

    const deleteButton = Array.from(container.querySelectorAll('button')).find(
      button => button.textContent === '删除',
    ) as HTMLButtonElement;

    await act(async () => {
      deleteButton.click();
      await Promise.resolve();
    });

    expect(window.confirm).toHaveBeenCalledWith('确认删除技能 Ping Skill？');
    expect(mockDeleteSkill).toHaveBeenCalledWith({ skill_id: 'skill-1' });
    expect(container.textContent).not.toContain('Ping Skill');

    act(() => {
      root?.unmount();
    });
    window.confirm = originalConfirm;
    container.remove();
  });

  it('clears stale test output when a later test run fails', async () => {
    const container = document.createElement('div');
    document.body.appendChild(container);
    let root: Root | undefined;

    mockTestRunSkill
      .mockResolvedValueOnce({
        data: { output: 'pong' },
        code: 0,
        msg: '',
      })
      .mockRejectedValueOnce(new Error('boom'));

    await act(async () => {
      root = createRoot(container);
      root.render(<SkillPage />);
      await Promise.resolve();
    });

    const moreButton = container.querySelector(
      'button[aria-label="更多 Ping Skill"]',
    ) as HTMLButtonElement;

    await act(async () => {
      moreButton.click();
      await Promise.resolve();
    });

    const runButton = Array.from(container.querySelectorAll('button')).find(
      button => button.textContent?.includes('试运行'),
    ) as HTMLButtonElement;

    await act(async () => {
      runButton.click();
      await Promise.resolve();
    });

    expect(container.textContent).toContain('pong');

    await act(async () => {
      runButton.click();
      await Promise.resolve();
    });

    expect(container.textContent).toContain('boom');
    expect(container.textContent).not.toContain('pong');

    act(() => {
      root?.unmount();
    });
    container.remove();
  });
});
