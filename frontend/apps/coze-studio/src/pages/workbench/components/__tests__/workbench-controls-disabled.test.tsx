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

/* eslint-disable @typescript-eslint/naming-convention -- Test doubles preserve external PascalCase component and icon exports. */

import {
  type ButtonHTMLAttributes,
  type InputHTMLAttributes,
  type ReactElement,
  type ReactNode,
} from 'react';

import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import { act, Simulate } from 'react-dom/test-utils';
import { createRoot, type Root } from 'react-dom/client';

const mockNavigate = vi.hoisted(() => vi.fn());
const mockListSkills = vi.hoisted(() => vi.fn());
const mockListMCPTools = vi.hoisted(() => vi.fn());

vi.mock('react-router-dom', () => ({
  useNavigate: () => mockNavigate,
  useParams: () => ({ space_id: 'space-1' }),
}));

vi.mock('../../../skill/service', () => ({
  listSkills: mockListSkills,
}));

vi.mock('../../../tools/service', () => ({
  listMCPToolRegistryEntries: mockListMCPTools,
}));

vi.mock('../../service', () => ({
  getWorkbenchLLMModels: vi.fn(),
  listWorkbenchDatabaseResources: vi.fn().mockResolvedValue([]),
  listWorkbenchKnowledgeResources: vi.fn().mockResolvedValue([]),
  listWorkbenchWorkflowResources: vi.fn().mockResolvedValue([]),
}));

vi.mock('@coze-arch/coze-design', () => ({
  Button: ({
    children,
    icon,
    iconPosition: _iconPosition,
    loading: _loading,
    size: _size,
    theme: _theme,
    type: _designType,
    ...props
  }: ButtonHTMLAttributes<HTMLButtonElement> & {
    icon?: ReactNode;
    iconPosition?: string;
    loading?: boolean;
    size?: string;
    theme?: string;
    type?: string;
  }) => (
    <button type="button" {...props}>
      {icon}
      {children}
    </button>
  ),
  Input: ({
    onChange,
    prefix,
    showClear: _showClear,
    size: _size,
    ...props
  }: Omit<InputHTMLAttributes<HTMLInputElement>, 'onChange' | 'prefix'> & {
    onChange?: (value: string) => void;
    prefix?: ReactNode;
    showClear?: boolean;
    size?: string;
  }) => (
    <label>
      {prefix}
      <input {...props} onChange={event => onChange?.(event.target.value)} />
    </label>
  ),
  Modal: ({
    children,
    visible,
  }: {
    children?: ReactNode;
    visible?: boolean;
  }) => (visible ? <div>{children}</div> : null),
  Spin: () => <span>加载中</span>,
  Space: ({ children }: { children?: ReactNode }) => <div>{children}</div>,
  Tabs: ({
    activeKey,
    onChange,
    tabBarExtraContent,
    tabList,
  }: {
    activeKey?: string;
    onChange?: (key: string) => void;
    tabBarExtraContent?: ReactNode;
    tabList?: Array<{ itemKey: string; tab: ReactNode }>;
  }) => (
    <div>
      {tabList?.map(tab => (
        <button
          aria-pressed={activeKey === tab.itemKey}
          key={tab.itemKey}
          type="button"
          onClick={() => onChange?.(tab.itemKey)}
        >
          {tab.tab}
        </button>
      ))}
      {tabBarExtraContent}
    </div>
  ),
  Typography: {
    Text: ({ children }: { children?: ReactNode }) => <span>{children}</span>,
  },
}));

const Icon = ({ name }: { name: string }) => (
  <span aria-hidden="true" data-icon={name} />
);

vi.mock('@coze-arch/coze-design/icons', () => ({
  IconCozArrowBack: () => <Icon name="arrow-back" />,
  IconCozArrowDown: () => <Icon name="arrow-down" />,
  IconCozArrowRight: () => <Icon name="arrow-right" />,
  IconCozCheckMark: () => <Icon name="check" />,
  IconCozCode: () => <Icon name="code" />,
  IconCozCross: () => <Icon name="cross" />,
  IconCozDatabase: () => <Icon name="database" />,
  IconCozDiamondFill: () => <Icon name="diamond-fill" />,
  IconCozKnowledge: () => <Icon name="knowledge" />,
  IconCozLightbulbFill: () => <Icon name="lightbulb-fill" />,
  IconCozMagnifier: () => <Icon name="search" />,
  IconCozPlugin: () => <Icon name="plugin" />,
  IconCozSetting: () => <Icon name="setting" />,
  IconCozSkill: () => <Icon name="skill" />,
  IconCozWorkflow: () => <Icon name="workflow" />,
}));

vi.mock('../workbench-composer', () => ({
  WorkbenchComposer: () => <div aria-label="续聊输入框" />,
}));

import { WorkbenchRuntimeSettingsControl } from '../workbench-runtime-settings-control';
import { WorkbenchModelSelector } from '../workbench-model-selector';
import { AtMenu } from '../workbench-composer-at-menu';
import {
  createDefaultWorkbenchResourceSelection,
  createDefaultWorkbenchRuntimeSettings,
} from '../types';
import { ExtensionsPopover } from '../../extensions-popover';
import { TaskFollowUpComposer } from '../../../tasks/task-follow-up-composer';

(
  globalThis as { IS_REACT_ACT_ENVIRONMENT?: boolean }
).IS_REACT_ACT_ENVIRONMENT = true;

const mountedRoots: Array<{ container: HTMLDivElement; root: Root }> = [];

const renderElement = (element: ReactElement) => {
  const container = document.createElement('div');
  document.body.appendChild(container);
  const root = createRoot(container);
  mountedRoots.push({ container, root });
  act(() => root.render(element));

  return {
    container,
    rerender: (nextElement: ReactElement) =>
      act(() => root.render(nextElement)),
  };
};

afterEach(() => {
  mountedRoots.splice(0).forEach(({ container, root }) => {
    act(() => root.unmount());
    container.remove();
  });
});

beforeEach(() => {
  vi.clearAllMocks();
  mockListSkills.mockReturnValue(new Promise(() => undefined));
  mockListMCPTools.mockReturnValue(new Promise(() => undefined));
});

describe('Workbench editing controls disabled contract', () => {
  it('closes and disables an already-open extensions panel', () => {
    const onOpenChange = vi.fn();
    const props = {
      open: true,
      value: createDefaultWorkbenchResourceSelection(),
      onOpenChange,
    };
    const { container, rerender } = renderElement(
      <ExtensionsPopover {...props} />,
    );
    expect(
      container.querySelector('input[aria-label="搜索技能"]'),
    ).not.toBeNull();

    rerender(<ExtensionsPopover {...props} disabled />);

    expect(
      container.querySelector<HTMLButtonElement>('button[aria-label="拓展"]')
        ?.disabled,
    ).toBe(true);
    expect(container.querySelector('input[aria-label="搜索技能"]')).toBeNull();
    expect(
      container.querySelector('.chat-workbench-extension-footer'),
    ).toBeNull();
  });

  it('closes model search and blocks selection when disabled', () => {
    const onChange = vi.fn();
    const onOpenChange = vi.fn();
    const model = {
      model_type: 1,
      name: 'test-model',
      display_name: 'Test Model',
    };
    const { container } = renderElement(
      <WorkbenchModelSelector
        disabled
        loading={false}
        models={[model]}
        open
        value={1}
        onChange={onChange}
        onOpenChange={onOpenChange}
      />,
    );

    expect(
      container.querySelector<HTMLButtonElement>(
        'button[aria-label="选择模型"]',
      )?.disabled,
    ).toBe(true);
    expect(container.querySelector('input[aria-label="搜索模型"]')).toBeNull();
    expect(onChange).not.toHaveBeenCalled();
  });

  it('closes runtime settings when disabled', () => {
    const settings = createDefaultWorkbenchRuntimeSettings(
      createDefaultWorkbenchResourceSelection(),
    );
    const { container, rerender } = renderElement(
      <WorkbenchRuntimeSettingsControl
        settings={settings}
        onChange={vi.fn()}
      />,
    );
    const trigger = container.querySelector<HTMLButtonElement>(
      'button[aria-label="运行设置"]',
    );
    act(() => Simulate.click(trigger as HTMLButtonElement));
    expect(
      container.querySelector('[aria-label="运行设置"] + *'),
    ).not.toBeNull();

    rerender(
      <WorkbenchRuntimeSettingsControl
        disabled
        settings={settings}
        onChange={vi.fn()}
      />,
    );

    expect(
      container.querySelector<HTMLButtonElement>(
        'button[aria-label="运行设置"]',
      )?.disabled,
    ).toBe(true);
    expect(container.querySelector('.chat-workbench-runtime-panel')).toBeNull();
  });

  it('disables an open at menu and ignores keyboard selection', () => {
    const onResourceTypeSelect = vi.fn();
    const { container } = renderElement(
      <AtMenu
        disabled
        draft={{ stage: 'resource-type', query: '' }}
        placement="top"
        value={createDefaultWorkbenchResourceSelection()}
        onBackToResourceTypes={vi.fn()}
        onClose={vi.fn()}
        onReferenceSelect={vi.fn()}
        onResourceTypeSelect={onResourceTypeSelect}
      />,
    );

    expect(
      Array.from(container.querySelectorAll('button')).every(
        button => button.disabled,
      ),
    ).toBe(true);
    act(() => {
      document.dispatchEvent(
        new KeyboardEvent('keydown', { key: 'Enter', bubbles: true }),
      );
    });
    expect(onResourceTypeSelect).not.toHaveBeenCalled();
  });

  it('disables recommended follow-ups during submitting and streaming', () => {
    const baseProps = {
      error: '',
      mode: 'flash' as const,
      onDismissSuggestions: vi.fn(),
      onModeChange: vi.fn(),
      onSubmit: vi.fn(),
      onSuggestionClick: vi.fn(),
      onValueChange: vi.fn(),
      suggestions: ['继续分析'],
      value: '',
    };
    const { container, rerender } = renderElement(
      <TaskFollowUpComposer {...baseProps} loading />,
    );

    expect(
      container.querySelector<HTMLButtonElement>(
        'button.coze-prototype-followup-suggestion',
      )?.disabled,
    ).toBe(true);
    expect(
      container.querySelector<HTMLButtonElement>(
        'button[aria-label="关闭推荐追问"]',
      )?.disabled,
    ).toBe(true);

    rerender(<TaskFollowUpComposer {...baseProps} loading={false} stopMode />);
    expect(
      container.querySelector<HTMLButtonElement>(
        'button.coze-prototype-followup-suggestion',
      )?.disabled,
    ).toBe(true);
  });

  it('renders library icons instead of visible pseudo-icon glyphs', () => {
    const model = {
      model_type: 1,
      name: 'test-model',
      display_name: 'Test Model',
    };
    const { container } = renderElement(
      <>
        <ExtensionsPopover
          open
          value={createDefaultWorkbenchResourceSelection()}
        />
        <WorkbenchModelSelector
          loading={false}
          models={[model]}
          open
          value={1}
          onChange={vi.fn()}
          onOpenChange={vi.fn()}
        />
        <AtMenu
          draft={{ stage: 'resource-type', query: '' }}
          placement="top"
          value={createDefaultWorkbenchResourceSelection()}
          onBackToResourceTypes={vi.fn()}
          onClose={vi.fn()}
          onReferenceSelect={vi.fn()}
          onResourceTypeSelect={vi.fn()}
        />
      </>,
    );

    expect(container.textContent).not.toMatch(/[✦‹›✓⌕↵↑↓]/);
    expect(container.textContent).not.toContain('</>');
    expect(container.querySelectorAll('[data-icon]').length).toBeGreaterThan(0);
  });
});
