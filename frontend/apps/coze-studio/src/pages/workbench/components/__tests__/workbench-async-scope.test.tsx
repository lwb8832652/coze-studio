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
  useState,
} from 'react';

import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import { act, Simulate } from 'react-dom/test-utils';
import { createRoot, type Root } from 'react-dom/client';

const mockSpace = vi.hoisted(() => ({ current: 'space-old' }));
const mockListDatabaseResources = vi.hoisted(() => vi.fn());
const mockListKnowledgeResources = vi.hoisted(() => vi.fn());
const mockListWorkflowResources = vi.hoisted(() => vi.fn());
const mockListSkills = vi.hoisted(() => vi.fn());
const mockListMCPTools = vi.hoisted(() => vi.fn());

vi.mock('react-router-dom', () => ({
  useNavigate: () => vi.fn(),
  useParams: () => ({ space_id: mockSpace.current }),
}));

vi.mock('../../service', () => ({
  listWorkbenchDatabaseResources: mockListDatabaseResources,
  listWorkbenchKnowledgeResources: mockListKnowledgeResources,
  listWorkbenchWorkflowResources: mockListWorkflowResources,
}));

vi.mock('../../../skill/service', () => ({
  listSkills: mockListSkills,
}));

vi.mock('../../../tools/service', () => ({
  listMCPToolRegistryEntries: mockListMCPTools,
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
  Space: ({ children }: { children?: ReactNode }) => <div>{children}</div>,
  Spin: () => <span>加载中</span>,
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
  TextArea: ({
    autosize: _autosize,
    onChange,
    ...props
  }: Omit<React.TextareaHTMLAttributes<HTMLTextAreaElement>, 'onChange'> & {
    autosize?: boolean | { minRows: number; maxRows: number };
    onChange?: (value: string) => void;
  }) => (
    <textarea {...props} onChange={event => onChange?.(event.target.value)} />
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
  IconCozArrowUp: () => <Icon name="arrow-up" />,
  IconCozAt: () => <Icon name="at" />,
  IconCozCheckMark: () => <Icon name="check" />,
  IconCozCode: () => <Icon name="code" />,
  IconCozCross: () => <Icon name="cross" />,
  IconCozDatabase: () => <Icon name="database" />,
  IconCozDiamondFill: () => <Icon name="diamond-fill" />,
  IconCozKnowledge: () => <Icon name="knowledge" />,
  IconCozLightbulbFill: () => <Icon name="lightbulb-fill" />,
  IconCozLightbulb: () => <Icon name="lightbulb" />,
  IconCozLightningFill: () => <Icon name="lightning" />,
  IconCozLink: () => <Icon name="link" />,
  IconCozMagnifier: () => <Icon name="search" />,
  IconCozPlugin: () => <Icon name="plugin" />,
  IconCozRocketFill: () => <Icon name="rocket" />,
  IconCozSendFill: () => <Icon name="send" />,
  IconCozSetting: () => <Icon name="setting" />,
  IconCozSkill: () => <Icon name="skill" />,
  IconCozStopCircle: () => <Icon name="stop" />,
  IconCozUpload: () => <Icon name="upload" />,
  IconCozWorkflow: () => <Icon name="workflow" />,
}));

vi.mock('../workbench-runtime-settings-control', () => ({
  WorkbenchRuntimeSettingsControl: () => null,
}));

import { WorkbenchModelSelector } from '../workbench-model-selector';
import { AtMenu } from '../workbench-composer-at-menu';
import { WorkbenchComposer } from '../workbench-composer';
import {
  createDefaultWorkbenchResourceSelection,
  type WorkbenchComposerSubmitPayload,
} from '../types';
import { ExtensionsPopover } from '../../extensions-popover';

(
  globalThis as { IS_REACT_ACT_ENVIRONMENT?: boolean }
).IS_REACT_ACT_ENVIRONMENT = true;

const mountedRoots: Array<{ container: HTMLDivElement; root: Root }> = [];

const createDeferred = <T,>() => {
  let resolve!: (value: T) => void;
  const promise = new Promise<T>(resolvePromise => {
    resolve = resolvePromise;
  });

  return { promise, resolve };
};

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

beforeEach(() => {
  vi.clearAllMocks();
  mockSpace.current = 'space-old';
  mockListMCPTools.mockReturnValue(new Promise(() => undefined));
  mockListSkills.mockReturnValue(new Promise(() => undefined));
  mockListWorkflowResources.mockResolvedValue([]);
  window.localStorage.clear();
});

afterEach(() => {
  mountedRoots.splice(0).forEach(({ container, root }) => {
    act(() => root.unmount());
    container.remove();
  });
});

describe('Workbench async resource scopes', () => {
  it('binds AtMenu results to space and resource type generations', async () => {
    const knowledgeDeferred =
      createDeferred<
        Array<{ id: string; name: string; description: string }>
      >();
    const databaseDeferred =
      createDeferred<
        Array<{ id: string; name: string; description: string }>
      >();
    mockListKnowledgeResources.mockReturnValue(knowledgeDeferred.promise);
    mockListDatabaseResources.mockReturnValue(databaseDeferred.promise);
    const selection = createDefaultWorkbenchResourceSelection();
    const baseProps = {
      placement: 'bottom' as const,
      value: selection,
      onBackToResourceTypes: vi.fn(),
      onClose: vi.fn(),
      onReferenceSelect: vi.fn(),
      onResourceTypeSelect: vi.fn(),
    };
    const { container, rerender } = renderElement(
      <AtMenu
        {...baseProps}
        draft={{
          stage: 'resource-search',
          resourceType: '知识库',
          query: '',
        }}
        spaceId="space-old"
      />,
    );
    await act(async () => {
      await Promise.resolve();
    });

    rerender(
      <AtMenu
        {...baseProps}
        draft={{
          stage: 'resource-search',
          resourceType: '数据库',
          query: '',
        }}
        spaceId="space-new"
      />,
    );
    expect(container.textContent).not.toContain('旧知识库');

    await act(async () => {
      knowledgeDeferred.resolve([
        { id: 'kb-old', name: '旧知识库', description: 'old' },
      ]);
      await Promise.resolve();
    });
    expect(container.textContent).not.toContain('旧知识库');

    await act(async () => {
      databaseDeferred.resolve([
        { id: 'db-new', name: '新数据库', description: 'new' },
      ]);
      await Promise.resolve();
    });
    expect(container.textContent).toContain('新数据库');
    expect(mockListKnowledgeResources).toHaveBeenCalledWith('space-old');
    expect(mockListDatabaseResources).toHaveBeenCalledWith('space-new');
  });

  it('does not expose an old ExtensionsPopover scope while the new space loads', async () => {
    const nextSkillsDeferred = createDeferred<{
      data: {
        skills: Array<{
          id: string;
          name: string;
          description: string;
          enabled: boolean;
        }>;
      };
    }>();
    mockListSkills
      .mockResolvedValueOnce({
        data: {
          skills: [
            {
              id: 'skill-old',
              name: '旧空间技能',
              description: 'old',
              enabled: true,
            },
          ],
        },
      })
      .mockReturnValueOnce(nextSkillsDeferred.promise);
    const props = {
      open: true,
      value: createDefaultWorkbenchResourceSelection(),
    };
    const { container, rerender } = renderElement(
      <ExtensionsPopover {...props} />,
    );
    await act(async () => {
      await Promise.resolve();
      await Promise.resolve();
    });
    expect(container.textContent).toContain('旧空间技能');

    mockSpace.current = 'space-new';
    rerender(<ExtensionsPopover {...props} />);
    expect(container.textContent).not.toContain('旧空间技能');

    await act(async () => {
      nextSkillsDeferred.resolve({
        data: {
          skills: [
            {
              id: 'skill-new',
              name: '新空间技能',
              description: 'new',
              enabled: true,
            },
          ],
        },
      });
      await Promise.resolve();
    });
    expect(container.textContent).toContain('新空间技能');
  });

  it('never submits a model from the previous space while the new scope loads', async () => {
    const oldModels =
      createDeferred<
        Array<{ model_type: number; name: string; display_name: string }>
      >();
    const newModels =
      createDeferred<
        Array<{ model_type: number; name: string; display_name: string }>
      >();
    const modelLoader = vi.fn((spaceId: string) =>
      spaceId === 'space-old' ? oldModels.promise : newModels.promise,
    );
    const payloads: WorkbenchComposerSubmitPayload[] = [];

    const Harness = () => {
      const [spaceId, setSpaceId] = useState('space-old');
      const [value, setValue] = useState('提交内容');

      return (
        <>
          <button
            aria-label="切换空间"
            type="button"
            onClick={() => setSpaceId('space-new')}
          />
          <WorkbenchComposer
            loading={false}
            mode="pro"
            modelLoader={modelLoader}
            spaceId={spaceId}
            value={value}
            onModeChange={vi.fn()}
            onSubmit={payload => payloads.push(payload)}
            onValueChange={setValue}
          />
        </>
      );
    };
    const { container } = renderElement(<Harness />);

    await act(async () => {
      oldModels.resolve([
        { model_type: 1, name: 'old-model', display_name: '旧空间模型' },
      ]);
      await Promise.resolve();
      await Promise.resolve();
    });
    expect(container.textContent).toContain('旧空间模型');

    act(() => {
      Simulate.click(
        container.querySelector<HTMLButtonElement>(
          'button[aria-label="切换空间"]',
        ) as HTMLButtonElement,
      );
    });
    act(() => {
      Simulate.keyDown(
        container.querySelector('textarea') as HTMLTextAreaElement,
        { key: 'Enter', code: 'Enter' },
      );
    });
    expect(payloads[0]?.modelType).toBeUndefined();
    expect(payloads[0]?.modelName).toBeUndefined();
    expect(container.textContent).not.toContain('旧空间模型');

    await act(async () => {
      newModels.resolve([
        { model_type: 2, name: 'new-model', display_name: '新空间模型' },
      ]);
      await Promise.resolve();
      await Promise.resolve();
    });
    act(() => {
      Simulate.keyDown(
        container.querySelector('textarea') as HTMLTextAreaElement,
        { key: 'Enter', code: 'Enter' },
      );
    });
    expect(payloads[1]).toEqual(
      expect.objectContaining({
        modelType: 2,
        modelName: 'new-model',
      }),
    );
  });

  it('refreshes the workspace model list whenever the selector opens', async () => {
    const modelLoader = vi
      .fn()
      .mockResolvedValueOnce([
        { model_type: 1, name: 'old-model', display_name: '旧模型' },
      ])
      .mockResolvedValueOnce([
        { model_type: 2, name: 'new-model', display_name: '新模型' },
      ]);
    const { container } = renderElement(
      <WorkbenchComposer
        loading={false}
        mode="pro"
        modelLoader={modelLoader}
        spaceId="space-old"
        value=""
        onModeChange={vi.fn()}
        onSubmit={vi.fn()}
        onValueChange={vi.fn()}
      />,
    );

    await act(async () => {
      await Promise.resolve();
      await Promise.resolve();
    });
    expect(container.textContent).toContain('旧模型');

    await act(async () => {
      Simulate.click(
        container.querySelector<HTMLButtonElement>(
          'button[aria-label="选择模型"]',
        ) as HTMLButtonElement,
      );
      await Promise.resolve();
      await Promise.resolve();
    });

    expect(modelLoader).toHaveBeenCalledTimes(2);
    expect(container.textContent).toContain('新模型');
    expect(container.textContent).not.toContain('旧模型');
  });
});

describe('Workbench overlay lifecycle', () => {
  it('limits AtMenu keyboard handling and exposes active descendant navigation', () => {
    const onResourceTypeSelect = vi.fn();
    const { container } = renderElement(
      <>
        <button aria-label="无关按钮" type="button" />
        <AtMenu
          draft={{ stage: 'resource-type', query: '' }}
          placement="bottom"
          value={createDefaultWorkbenchResourceSelection()}
          onBackToResourceTypes={vi.fn()}
          onClose={vi.fn()}
          onReferenceSelect={vi.fn()}
          onResourceTypeSelect={onResourceTypeSelect}
        />
      </>,
    );
    const unrelatedButton = container.querySelector<HTMLButtonElement>(
      'button[aria-label="无关按钮"]',
    );
    act(() => {
      unrelatedButton?.dispatchEvent(
        new KeyboardEvent('keydown', { key: 'Enter', bubbles: true }),
      );
    });
    expect(onResourceTypeSelect).not.toHaveBeenCalled();

    const menu = container.querySelector<HTMLElement>(
      '.chat-workbench-at-menu',
    );
    const firstActiveDescendant = menu?.getAttribute('aria-activedescendant');
    expect(firstActiveDescendant).toBeTruthy();
    act(() => {
      menu?.dispatchEvent(
        new KeyboardEvent('keydown', { key: 'ArrowDown', bubbles: true }),
      );
    });
    expect(menu?.getAttribute('aria-activedescendant')).not.toBe(
      firstActiveDescendant,
    );
  });

  it('closes extensions and model overlays on Escape/outside and restores trigger focus', async () => {
    const Harness = () => {
      const [extensionsOpen, setExtensionsOpen] = useState(true);
      const [modelOpen, setModelOpen] = useState(false);

      return (
        <>
          <button aria-label="外部区域" type="button" />
          <ExtensionsPopover
            open={extensionsOpen}
            renderMask={false}
            value={createDefaultWorkbenchResourceSelection()}
            onOpenChange={setExtensionsOpen}
          />
          <WorkbenchModelSelector
            loading={false}
            models={[
              {
                model_type: 1,
                name: 'model-1',
                display_name: '模型一',
              },
            ]}
            open={modelOpen}
            value={1}
            onChange={vi.fn()}
            onOpenChange={setModelOpen}
          />
        </>
      );
    };
    const { container } = renderElement(<Harness />);
    const extensionSearch = container.querySelector<HTMLInputElement>(
      'input[aria-label="搜索技能"]',
    );
    await act(async () => {
      extensionSearch?.dispatchEvent(
        new KeyboardEvent('keydown', { key: 'Escape', bubbles: true }),
      );
      await Promise.resolve();
    });
    expect(container.querySelector('input[aria-label="搜索技能"]')).toBeNull();
    expect(document.activeElement?.getAttribute('aria-label')).toBe('拓展');

    await act(async () => {
      Simulate.click(
        container.querySelector<HTMLButtonElement>(
          'button[aria-label="选择模型"]',
        ) as HTMLButtonElement,
      );
      await Promise.resolve();
    });
    const outside = container.querySelector<HTMLButtonElement>(
      'button[aria-label="外部区域"]',
    );
    await act(async () => {
      outside?.dispatchEvent(new MouseEvent('mousedown', { bubbles: true }));
      await Promise.resolve();
    });
    expect(container.querySelector('input[aria-label="搜索模型"]')).toBeNull();
    expect(document.activeElement?.getAttribute('aria-label')).toBe('选择模型');
  });
});
