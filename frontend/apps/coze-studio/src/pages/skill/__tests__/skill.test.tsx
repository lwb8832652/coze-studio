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
import { createRoot, type Root } from 'react-dom/client';
import { workbenchSkill } from '@coze-studio/api-schema';

globalThis.IS_REACT_ACT_ENVIRONMENT = true;

const mockUseParams = vi.hoisted(() => vi.fn(() => ({ space_id: 'space-1' })));
const mockListSkills = vi.hoisted(() => vi.fn());
const mockCreateSkill = vi.hoisted(() => vi.fn());
const mockDeleteSkill = vi.hoisted(() => vi.fn());
const mockTestRunSkill = vi.hoisted(() => vi.fn());
const mockUpdateSkill = vi.hoisted(() => vi.fn());
const mockListSkillToolCandidates = vi.hoisted(() => vi.fn());
const mockListSkillVersions = vi.hoisted(() => vi.fn());
const mockListSkillVersionResources = vi.hoisted(() => vi.fn());

vi.mock('react-router-dom', () => ({
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
  createSkill: mockCreateSkill,
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
  type: workbenchSkill.SkillType.Script,
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
    mockCreateSkill.mockReset();
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
    mockCreateSkill.mockResolvedValue({
      data: {
        ...skill,
        id: 'skill-created-1',
        name: 'Market Scan',
        description: 'Track competitor news',
        type: workbenchSkill.SkillType.CustomSkill,
      },
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

  it('renders the redesigned skill configuration structure', async () => {
    const container = document.createElement('div');
    document.body.appendChild(container);
    let root: Root | undefined;

    await act(async () => {
      root = createRoot(container);
      root.render(<SkillPage />);
      await Promise.resolve();
    });

    expect(container.textContent).toContain('技能配置');
    expect(container.textContent).toContain('Aime 专属助理准备好');
    expect(container.textContent).toContain('去聊天专属助理');
    expect(container.textContent).toContain(
      '集中管理工作空间内的全部技能,支持发布、订阅、调用与版本管理。',
    );
    expect(container.textContent).toContain('查看文档');
    expect(container.textContent).toContain('全部技能');
    expect(container.textContent).toContain('自定义');
    expect(container.textContent).toContain('公共');
    expect(container.textContent).toContain('内置');
    expect(container.textContent).toContain('脚本');
    expect(container.textContent).toContain('工作流');
    expect(container.textContent).toContain('创建技能');
    expect(
      container.querySelector('input[aria-label="搜索技能"]'),
    ).toBeTruthy();
    expect(container.textContent).toContain('已发布');

    act(() => {
      root?.unmount();
    });
    container.remove();
  });

  it('creates a custom skill from the basic creation form', async () => {
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
        data: {
          skills: [
            skill,
            {
              ...skill,
              id: 'skill-created-1',
              name: 'Market Scan',
              description: 'Track competitor news',
              type: workbenchSkill.SkillType.CustomSkill,
            },
          ],
        },
        code: 0,
        msg: '',
      });

    await act(async () => {
      root = createRoot(container);
      root.render(<SkillPage />);
      await Promise.resolve();
    });

    const createButton = Array.from(container.querySelectorAll('button')).find(
      button => button.textContent === '创建技能',
    ) as HTMLButtonElement;

    await act(async () => {
      createButton.click();
      await Promise.resolve();
    });

    const nameInputElement = container.querySelector(
      'input[aria-label="新技能名称"]',
    );
    const descriptionInputElement = container.querySelector(
      'textarea[aria-label="新技能描述"]',
    );

    expect(nameInputElement).toBeTruthy();
    expect(descriptionInputElement).toBeTruthy();
    const nameInput = nameInputElement as HTMLInputElement;
    const descriptionInput = descriptionInputElement as HTMLTextAreaElement;

    await act(async () => {
      Simulate.change(nameInput, {
        target: { value: 'Market Scan' },
      } as unknown as Event);
      Simulate.change(descriptionInput, {
        target: { value: 'Track competitor news' },
      } as unknown as Event);
      await Promise.resolve();
    });

    const submitButton = Array.from(container.querySelectorAll('button')).find(
      button => button.textContent === '保存技能',
    ) as HTMLButtonElement;
    expect(submitButton).toBeTruthy();
    expect(submitButton.disabled).toBe(false);

    await act(async () => {
      submitButton.click();
      await Promise.resolve();
    });

    expect(mockCreateSkill).toHaveBeenCalledWith({
      space_id: 'space-1',
      name: 'Market Scan',
      description: 'Track competitor news',
      type: workbenchSkill.SkillType.CustomSkill,
      version: '1.0.0',
      enabled: true,
      input_schema: '{}',
      output_schema: '{}',
      executor: '{}',
      permissions: '{"network":false,"allowed_tools":[]}',
    });
    expect(container.textContent).toContain('Market Scan');

    act(() => {
      root?.unmount();
    });
    container.remove();
  });

  it('filters skills by custom public bootstrap and legacy categories', () => {
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
    const workflowSkill = {
      ...skill,
      id: 'skill-workflow',
      name: 'Workflow Skill',
      type: workbenchSkill.SkillType.Workflow,
    };
    const skills = [customSkill, publicSkill, bootstrapSkill, workflowSkill];

    expect(
      getVisibleSkills(skills, '', 'custom' as SkillTypeFilter).map(
        item => item.id,
      ),
    ).toEqual(['skill-custom']);
    expect(
      getVisibleSkills(skills, '', 'public' as SkillTypeFilter).map(
        item => item.id,
      ),
    ).toEqual(['skill-public']);
    expect(
      getVisibleSkills(skills, '', 'bootstrap' as SkillTypeFilter).map(
        item => item.id,
      ),
    ).toEqual(['skill-bootstrap']);
    expect(
      getVisibleSkills(skills, '', 'workflow' as SkillTypeFilter).map(
        item => item.id,
      ),
    ).toEqual(['skill-workflow']);
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

    const manageButton = container.querySelector(
      'button[aria-label="管理 Ping Skill"]',
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

    const disableButton = Array.from(container.querySelectorAll('button')).find(
      button => button.textContent === '停用',
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
      type: workbenchSkill.SkillType.Script,
      version: '1.0.0',
      enabled: false,
      input_schema: '{}',
      output_schema: '{}',
      executor: '{}',
      permissions: '{}',
    });
    expect(container.textContent).toContain('已停用');

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
