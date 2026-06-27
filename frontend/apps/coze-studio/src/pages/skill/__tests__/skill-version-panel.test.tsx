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

const mockListSkillVersions = vi.hoisted(() => vi.fn());
const mockListSkillVersionResources = vi.hoisted(() => vi.fn());
const mockListSkillToolCandidates = vi.hoisted(() => vi.fn());
const mockUpdateSkillVersionContent = vi.hoisted(() => vi.fn());
const mockUpdateSkillVersionResource = vi.hoisted(() => vi.fn());
const mockRollbackSkillVersion = vi.hoisted(() => vi.fn());
const mockExportSkillVersion = vi.hoisted(() => vi.fn());
const mockUpdateSkill = vi.hoisted(() => vi.fn());

/* eslint-disable @typescript-eslint/naming-convention -- Semi mock exports keep component names and aria props. */
vi.mock('@coze-arch/coze-design', () => ({
  Banner: ({ description }: { description?: ReactNode }) => (
    <div>{description}</div>
  ),
  Button: ({
    children,
    disabled,
    onClick,
  }: {
    children?: ReactNode;
    disabled?: boolean;
    onClick?: () => void;
  }) => (
    <button type="button" disabled={disabled} onClick={onClick}>
      {children}
    </button>
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
  Input: ({
    'aria-label': ariaLabel,
    onChange,
    value,
  }: {
    'aria-label'?: string;
    onChange?: (value: string) => void;
    value?: string;
  }) => (
    <input
      aria-label={ariaLabel}
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
    value,
  }: {
    'aria-label'?: string;
    onChange?: (value: string) => void;
    value?: string;
  }) => (
    <textarea
      aria-label={ariaLabel}
      value={value}
      onChange={event => onChange?.(event.target.value)}
    />
  ),
}));

vi.mock('@coze-arch/coze-design/icons', () => ({
  IconCozHistory: () => <span />,
}));
/* eslint-enable @typescript-eslint/naming-convention -- Restore naming checks after component mocks. */

vi.mock('../service', () => ({
  exportSkillVersion: mockExportSkillVersion,
  listSkillToolCandidates: mockListSkillToolCandidates,
  listSkillVersionResources: mockListSkillVersionResources,
  listSkillVersions: mockListSkillVersions,
  rollbackSkillVersion: mockRollbackSkillVersion,
  updateSkill: mockUpdateSkill,
  updateSkillVersionContent: mockUpdateSkillVersionContent,
  updateSkillVersionResource: mockUpdateSkillVersionResource,
}));

import { SkillVersionPanel } from '../skill-version-panel';

const skill = {
  id: 'skill-1',
  space_id: 'space-1',
  name: 'Research Skill',
  description: 'Research from trusted sources',
  type: workbenchSkill.SkillType.DeerSkill,
  version: '1.0.0',
  enabled: true,
  input_schema: '{}',
  output_schema: '{}',
  executor: '{}',
  permissions:
    '{"network":false,"allowed_tools":["search"],"sandbox":"readonly"}',
  created_at: 1717000000000,
  updated_at: 1717000300000,
};

const version = {
  id: 'version-1',
  skill_id: 'skill-1',
  version: '1.0.0',
  skill_md: '---\nname: Research Skill\n---\n\nOriginal instructions',
  input_schema: '{}',
  output_schema: '{}',
  executor: '{}',
  permissions: '{}',
  created_at: 1717000300000,
};

const resource = {
  id: 'resource-1',
  skill_id: 'skill-1',
  version_id: 'version-1',
  path: 'references/guide.md',
  content_base64: window.btoa('Original guide'),
  size: 14,
  sha256: 'hash',
  created_at: 1717000300000,
};

describe('SkillVersionPanel', () => {
  beforeEach(() => {
    mockListSkillVersions.mockReset();
    mockListSkillVersionResources.mockReset();
    mockListSkillToolCandidates.mockReset();
    mockUpdateSkillVersionContent.mockReset();
    mockUpdateSkillVersionResource.mockReset();
    mockRollbackSkillVersion.mockReset();
    mockExportSkillVersion.mockReset();
    mockUpdateSkill.mockReset();

    mockListSkillVersions.mockResolvedValue({
      data: { versions: [version] },
      code: 0,
      msg: '',
    });
    mockListSkillVersionResources.mockResolvedValue({
      data: { resources: [resource] },
      code: 0,
      msg: '',
    });
    mockListSkillToolCandidates.mockResolvedValue({
      data: {
        tools: [
          {
            name: 'web_fetch',
            display_name: 'Web fetch',
            description: 'Fetch bounded text content.',
            category: 'web',
            visibility: 'static',
          },
          {
            name: 'web_search',
            display_name: 'Web search',
            description: 'Search using an approved backend.',
            category: 'web',
            visibility: 'static',
          },
        ],
      },
      code: 0,
      msg: '',
    });
    mockUpdateSkillVersionContent.mockResolvedValue({
      data: version,
      code: 0,
      msg: '',
    });
    mockUpdateSkillVersionResource.mockResolvedValue({
      data: version,
      code: 0,
      msg: '',
    });
    mockRollbackSkillVersion.mockResolvedValue({
      data: skill,
      code: 0,
      msg: '',
    });
    mockUpdateSkill.mockResolvedValue({
      data: {
        ...skill,
        name: 'Research Copilot',
        description: 'Curated research assistant',
        version: '1.1.0',
      },
      code: 0,
      msg: '',
    });
  });

  it('updates base skill metadata from the version management panel', async () => {
    const container = document.createElement('div');
    document.body.appendChild(container);
    const onSkillChanged = vi.fn();
    let root: Root | undefined;

    await act(async () => {
      root = createRoot(container);
      root.render(
        <SkillVersionPanel
          skill={skill}
          onClose={vi.fn()}
          onSkillChanged={onSkillChanged}
        />,
      );
      await Promise.resolve();
    });

    const nameInputElement = container.querySelector(
      'input[aria-label="技能名称"]',
    );
    const versionInputElement = container.querySelector(
      'input[aria-label="技能版本"]',
    );
    const descriptionInputElement = container.querySelector(
      'textarea[aria-label="技能描述"]',
    );

    expect(nameInputElement).toBeTruthy();
    expect(versionInputElement).toBeTruthy();
    expect(descriptionInputElement).toBeTruthy();

    act(() => {
      Simulate.change(
        nameInputElement as HTMLInputElement,
        {
          target: { value: 'Research Copilot' },
        } as unknown as Event,
      );
      Simulate.change(
        descriptionInputElement as HTMLTextAreaElement,
        {
          target: { value: 'Curated research assistant' },
        } as unknown as Event,
      );
      Simulate.change(
        versionInputElement as HTMLInputElement,
        {
          target: { value: '1.1.0' },
        } as unknown as Event,
      );
    });

    const saveButton = Array.from(container.querySelectorAll('button')).find(
      button => button.textContent === '保存基础信息',
    ) as HTMLButtonElement;

    await act(async () => {
      saveButton.click();
      await Promise.resolve();
    });

    expect(mockUpdateSkill).toHaveBeenCalledWith({
      id: 'skill-1',
      space_id: 'space-1',
      name: 'Research Copilot',
      description: 'Curated research assistant',
      type: workbenchSkill.SkillType.DeerSkill,
      version: '1.1.0',
      enabled: true,
      input_schema: '{}',
      output_schema: '{}',
      executor: '{}',
      permissions:
        '{"network":false,"allowed_tools":["search"],"sandbox":"readonly"}',
    });
    expect(onSkillChanged).toHaveBeenCalled();
    expect(container.textContent).toContain('基础信息已保存');

    act(() => {
      root?.unmount();
    });
    container.remove();
  });

  it('updates skill permissions while preserving unknown permission keys', async () => {
    const container = document.createElement('div');
    document.body.appendChild(container);
    const onSkillChanged = vi.fn();
    let root: Root | undefined;

    await act(async () => {
      root = createRoot(container);
      root.render(
        <SkillVersionPanel
          skill={skill}
          onClose={vi.fn()}
          onSkillChanged={onSkillChanged}
        />,
      );
      await Promise.resolve();
    });

    const networkCheckbox = container.querySelector(
      'input[aria-label="允许网络访问"]',
    ) as HTMLInputElement;
    const allowedToolsEditor = container.querySelector(
      'textarea[aria-label="允许工具名称"]',
    ) as HTMLTextAreaElement;

    expect(networkCheckbox.checked).toBe(false);
    expect(allowedToolsEditor.value).toBe('search');

    act(() => {
      Simulate.change(networkCheckbox, {
        target: { checked: true },
      } as unknown as Event);
      Simulate.change(allowedToolsEditor, {
        target: { value: 'search\nweb_fetch\nsearch' },
      } as unknown as Event);
    });

    const saveButton = Array.from(container.querySelectorAll('button')).find(
      button => button.textContent === '保存权限配置',
    ) as HTMLButtonElement;

    await act(async () => {
      saveButton.click();
      await Promise.resolve();
    });

    const updateRequest = mockUpdateSkill.mock.calls.at(-1)?.[0];
    expect(updateRequest).toMatchObject({
      id: 'skill-1',
      space_id: 'space-1',
      name: 'Research Skill',
      description: 'Research from trusted sources',
      type: workbenchSkill.SkillType.DeerSkill,
      version: '1.0.0',
      enabled: true,
      input_schema: '{}',
      output_schema: '{}',
      executor: '{}',
    });
    expect(JSON.parse(updateRequest.permissions)).toEqual({
      network: true,
      allowed_tools: ['search', 'web_fetch'],
      sandbox: 'readonly',
    });
    expect(onSkillChanged).toHaveBeenCalled();
    expect(container.textContent).toContain('权限配置已保存');

    act(() => {
      root?.unmount();
    });
    container.remove();
  });

  it('loads tool candidates and toggles a candidate into permissions', async () => {
    const container = document.createElement('div');
    document.body.appendChild(container);
    let root: Root | undefined;

    await act(async () => {
      root = createRoot(container);
      root.render(
        <SkillVersionPanel
          skill={skill}
          onClose={vi.fn()}
          onSkillChanged={vi.fn()}
        />,
      );
      await Promise.resolve();
    });

    expect(mockListSkillToolCandidates).toHaveBeenCalledWith({
      space_id: 'space-1',
    });

    const webFetchButton = Array.from(
      container.querySelectorAll('button'),
    ).find(button => button.textContent === 'Web fetch') as HTMLButtonElement;
    expect(webFetchButton).toBeTruthy();

    await act(async () => {
      webFetchButton.click();
      await Promise.resolve();
    });

    const allowedToolsEditor = container.querySelector(
      'textarea[aria-label="允许工具名称"]',
    ) as HTMLTextAreaElement;
    expect(allowedToolsEditor.value).toBe('search\nweb_fetch');

    act(() => {
      root?.unmount();
    });
    container.remove();
  });

  it('renders MCP candidate source and toggles the safe grant name', async () => {
    mockListSkillToolCandidates.mockResolvedValueOnce({
      data: {
        tools: [
          {
            name: 'mcp_100_search_docs',
            display_name: 'Search docs',
            description: 'Search internal documentation.',
            category: 'mcp',
            visibility: 'static',
            source: 'mcp',
            source_id: '100',
            source_name: 'docs-mcp',
          },
        ],
      },
      code: 0,
      msg: '',
    });

    const container = document.createElement('div');
    document.body.appendChild(container);
    let root: Root | undefined;

    await act(async () => {
      root = createRoot(container);
      root.render(
        <SkillVersionPanel
          skill={skill}
          onClose={vi.fn()}
          onSkillChanged={vi.fn()}
        />,
      );
      await Promise.resolve();
    });

    const mcpButton = Array.from(container.querySelectorAll('button')).find(
      button => {
        const text = button.textContent ?? '';

        return text.includes('Search docs') && text.includes('docs-mcp');
      },
    ) as HTMLButtonElement;
    expect(mcpButton).toBeTruthy();

    await act(async () => {
      mcpButton.click();
      await Promise.resolve();
    });

    const allowedToolsEditor = container.querySelector(
      'textarea[aria-label="允许工具名称"]',
    ) as HTMLTextAreaElement;
    expect(allowedToolsEditor.value).toBe('search\nmcp_100_search_docs');

    act(() => {
      root?.unmount();
    });
    container.remove();
  });

  it('rejects unsafe allowed tool names before updating permissions', async () => {
    const container = document.createElement('div');
    document.body.appendChild(container);
    let root: Root | undefined;

    await act(async () => {
      root = createRoot(container);
      root.render(
        <SkillVersionPanel
          skill={skill}
          onClose={vi.fn()}
          onSkillChanged={vi.fn()}
        />,
      );
      await Promise.resolve();
    });

    const allowedToolsEditor = container.querySelector(
      'textarea[aria-label="允许工具名称"]',
    ) as HTMLTextAreaElement;
    act(() => {
      Simulate.change(allowedToolsEditor, {
        target: { value: 'bad-name' },
      } as unknown as Event);
    });

    const saveButton = Array.from(container.querySelectorAll('button')).find(
      button => button.textContent === '保存权限配置',
    ) as HTMLButtonElement;

    await act(async () => {
      saveButton.click();
      await Promise.resolve();
    });

    expect(mockUpdateSkill).not.toHaveBeenCalled();
    expect(container.textContent).toContain('工具名称只能包含字母');

    act(() => {
      root?.unmount();
    });
    container.remove();
  });

  it('loads versions and saves SKILL.md as a snapshot', async () => {
    const container = document.createElement('div');
    document.body.appendChild(container);
    const onSkillChanged = vi.fn();
    let root: Root | undefined;

    await act(async () => {
      root = createRoot(container);
      root.render(
        <SkillVersionPanel
          skill={skill}
          onClose={vi.fn()}
          onSkillChanged={onSkillChanged}
        />,
      );
      await Promise.resolve();
    });

    const editor = container.querySelector(
      'textarea[aria-label="SKILL.md 内容"]',
    ) as HTMLTextAreaElement;
    expect(editor.value).toContain('Original instructions');

    act(() => {
      Simulate.change(editor, {
        target: {
          value: '---\nname: Research Skill\n---\n\nUpdated instructions',
        },
      } as unknown as Event);
    });

    const saveButton = Array.from(container.querySelectorAll('button')).find(
      button => button.textContent === '保存入口文件',
    ) as HTMLButtonElement;

    await act(async () => {
      saveButton.click();
      await Promise.resolve();
    });

    expect(mockUpdateSkillVersionContent).toHaveBeenCalledWith({
      skill_id: 'skill-1',
      version_id: 'version-1',
      skill_md: '---\nname: Research Skill\n---\n\nUpdated instructions',
    });
    expect(onSkillChanged).toHaveBeenCalled();
    expect(container.textContent).toContain('已保存为新版本快照');

    act(() => {
      root?.unmount();
    });
    container.remove();
  });

  it('encodes text resource edits before creating a new snapshot', async () => {
    const container = document.createElement('div');
    document.body.appendChild(container);
    let root: Root | undefined;

    await act(async () => {
      root = createRoot(container);
      root.render(
        <SkillVersionPanel
          skill={skill}
          onClose={vi.fn()}
          onSkillChanged={vi.fn()}
        />,
      );
      await Promise.resolve();
    });

    const editor = container.querySelector(
      'textarea[aria-label="资源内容"]',
    ) as HTMLTextAreaElement;
    expect(editor.value).toBe('Original guide');

    act(() => {
      Simulate.change(editor, {
        target: { value: 'Updated guide' },
      } as unknown as Event);
    });

    const saveButton = Array.from(container.querySelectorAll('button')).find(
      button => button.textContent === '保存资源',
    ) as HTMLButtonElement;

    await act(async () => {
      saveButton.click();
      await Promise.resolve();
    });

    expect(mockUpdateSkillVersionResource).toHaveBeenCalledWith({
      skill_id: 'skill-1',
      version_id: 'version-1',
      path: 'references/guide.md',
      content_base64: window.btoa('Updated guide'),
    });

    act(() => {
      root?.unmount();
    });
    container.remove();
  });
});
