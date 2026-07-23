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

import { flushSync } from 'react-dom';
import {
  type ButtonHTMLAttributes,
  type InputHTMLAttributes,
  type ReactNode,
  useState,
} from 'react';

import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import { act } from 'react-dom/test-utils';
import { createRoot, type Root } from 'react-dom/client';

vi.mock('react-router-dom', () => ({
  useNavigate: () => vi.fn(),
  useParams: () => ({}),
}));

vi.mock('../../service', () => ({
  listWorkbenchDatabaseResources: vi.fn(),
  listWorkbenchKnowledgeResources: vi.fn(),
  listWorkbenchWorkflowResources: vi.fn(),
}));

vi.mock('../../../tools/service', () => ({
  listMCPToolRegistryEntries: vi.fn(),
}));

vi.mock('lottie-web', () => ({
  default: {
    loadAnimation: vi.fn(() => ({
      addEventListener: vi.fn(),
      destroy: vi.fn(),
      removeEventListener: vi.fn(),
    })),
  },
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
  Tabs: () => null,
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

vi.mock('@coze-arch/coze-design/icons', () => {
  const MockIcon = () => <span aria-hidden="true" />;

  return {
    IconCozArrowBack: MockIcon,
    IconCozArrowDown: MockIcon,
    IconCozArrowRight: MockIcon,
    IconCozArrowUp: MockIcon,
    IconCozAt: MockIcon,
    IconCozCheckMark: MockIcon,
    IconCozCode: MockIcon,
    IconCozCross: MockIcon,
    IconCozDatabase: MockIcon,
    IconCozDiamondFill: MockIcon,
    IconCozKnowledge: MockIcon,
    IconCozLightbulb: MockIcon,
    IconCozLightbulbFill: MockIcon,
    IconCozLightningFill: MockIcon,
    IconCozLink: MockIcon,
    IconCozMagnifier: MockIcon,
    IconCozPlugin: MockIcon,
    IconCozRocketFill: MockIcon,
    IconCozSendFill: MockIcon,
    IconCozSetting: MockIcon,
    IconCozSkill: MockIcon,
    IconCozStopCircle: MockIcon,
    IconCozUpload: MockIcon,
    IconCozWorkflow: MockIcon,
  };
});

vi.mock('../workbench-runtime-settings-control', () => ({
  WorkbenchRuntimeSettingsControl: () => null,
}));

import { WorkbenchModelSelector } from '../workbench-model-selector';
import { WorkbenchComposer } from '../workbench-composer';
import {
  DEFAULT_WORKBENCH_MODE,
  type WorkbenchComposerSubmitPayload,
  type WorkbenchLLMModel,
} from '../types';
import { listSkills } from '../../../skill/service';

vi.mock('../../../skill/service', () => ({
  listSkills: vi.fn(),
}));

const deferred = <T,>() => {
  let resolve!: (value: T) => void;
  const promise = new Promise<T>(nextResolve => {
    resolve = nextResolve;
  });
  return { promise, resolve };
};

const skillResponse = (id: string, name: string) =>
  ({
    data: {
      skills: [{ id, name, enabled: true }],
    },
    code: 0,
    msg: '',
  }) as Awaited<ReturnType<typeof listSkills>>;

const SlashComposer = ({ spaceId }: { spaceId: string }) => {
  const [value, setValue] = useState('/');

  return (
    <WorkbenchComposer
      value={value}
      mode={DEFAULT_WORKBENCH_MODE}
      loading={false}
      spaceId={spaceId}
      onValueChange={setValue}
      onModeChange={vi.fn()}
      onSubmit={vi.fn()}
    />
  );
};

describe('Workbench composer final scope contracts', () => {
  let container: HTMLDivElement;
  let root: Root;

  beforeEach(() => {
    globalThis.IS_REACT_ACT_ENVIRONMENT = true;
    vi.clearAllMocks();
    container = document.createElement('div');
    document.body.appendChild(container);
    root = createRoot(container);
    window.localStorage.clear();
  });

  afterEach(() => {
    act(() => root.unmount());
    container.remove();
  });

  it('hides slash skills synchronously and ignores a late prior-space result', async () => {
    const spaceA = deferred<Awaited<ReturnType<typeof listSkills>>>();
    const spaceB = deferred<Awaited<ReturnType<typeof listSkills>>>();
    const spaceC = deferred<Awaited<ReturnType<typeof listSkills>>>();
    vi.mocked(listSkills).mockImplementation(({ space_id }) => {
      if (space_id === 'space-a') {
        return spaceA.promise;
      }
      if (space_id === 'space-b') {
        return spaceB.promise;
      }
      return spaceC.promise;
    });

    await act(async () => {
      root.render(<SlashComposer spaceId="space-a" />);
      await Promise.resolve();
    });
    const textarea = container.querySelector<HTMLTextAreaElement>(
      'textarea[aria-label="任务描述"]',
    )!;
    act(() => textarea.focus());

    await act(async () => {
      spaceA.resolve(skillResponse('skill-a', 'Space A Skill'));
      await spaceA.promise;
    });
    expect(container.textContent).toContain('Space A Skill');

    act(() => root.render(<SlashComposer spaceId="space-b" />));
    expect(container.textContent).not.toContain('Space A Skill');

    act(() => root.render(<SlashComposer spaceId="space-c" />));
    await act(async () => {
      spaceB.resolve(skillResponse('skill-b', 'Late Space B Skill'));
      await spaceB.promise;
    });
    expect(container.textContent).not.toContain('Late Space B Skill');

    await act(async () => {
      spaceC.resolve(skillResponse('skill-c', 'Space C Skill'));
      await spaceC.promise;
    });
    expect(container.textContent).toContain('Space C Skill');
  });

  it('restores model trigger focus for selection, Escape, and outside close', async () => {
    const model = {
      model_type: 1,
      name: 'scope-model',
      display_name: 'Scope Model',
      model_class_name: 'Test',
    } as WorkbenchLLMModel;

    const ModelHarness = () => {
      const [open, setOpen] = useState(false);

      return (
        <>
          <button type="button" aria-label="模型外部按钮">
            Outside
          </button>
          <WorkbenchModelSelector
            loading={false}
            models={[model]}
            open={open}
            value={1}
            onChange={vi.fn()}
            onOpenChange={setOpen}
          />
        </>
      );
    };

    act(() => root.render(<ModelHarness />));
    const trigger = container.querySelector<HTMLButtonElement>(
      'button[aria-label="选择模型"]',
    )!;
    const openAndExpectSearchFocus = async () => {
      await act(async () => {
        trigger.click();
        await Promise.resolve();
      });
      const search = container.querySelector<HTMLInputElement>(
        'input[aria-label="搜索模型"]',
      )!;
      expect(document.activeElement).toBe(search);
    };
    const expectTriggerFocus = async () => {
      await act(async () => {
        await Promise.resolve();
      });
      expect(document.activeElement).toBe(trigger);
    };

    await openAndExpectSearchFocus();
    act(() =>
      document.body.dispatchEvent(
        new MouseEvent('mousedown', { bubbles: true }),
      ),
    );
    await expectTriggerFocus();

    await openAndExpectSearchFocus();
    act(() =>
      container
        .querySelector<HTMLButtonElement>('button[role="option"]')!
        .click(),
    );
    await expectTriggerFocus();

    await openAndExpectSearchFocus();
    act(() =>
      document.dispatchEvent(
        new KeyboardEvent('keydown', { bubbles: true, key: 'Escape' }),
      ),
    );
    await expectTriggerFocus();

    await openAndExpectSearchFocus();
    act(() =>
      container
        .querySelector('button[aria-label="模型外部按钮"]')!
        .dispatchEvent(new MouseEvent('mousedown', { bubbles: true })),
    );
    await expectTriggerFocus();
    expect(document.activeElement).not.toBe(document.body);
  });

  it('blocks an old persistent resource selection before the new space scope is ready', async () => {
    const storedUsage = (skillId: string) =>
      JSON.stringify({
        version: 1,
        resourceSelection: {
          enable_skills: [skillId],
          explicit_enable_skills: [],
          enable_mcp: [],
          enable_kbs: [],
          enable_databases: [],
        },
        skillsEnabled: true,
        mcpToolsEnabled: true,
      });
    window.localStorage.setItem(
      'coze-workbench-extension-usage:space-a',
      storedUsage('persistent-a'),
    );
    window.localStorage.setItem(
      'coze-workbench-extension-usage:space-b',
      storedUsage('persistent-b'),
    );
    const onSubmit = vi.fn<(payload: WorkbenchComposerSubmitPayload) => void>();
    const renderComposer = (spaceId: string) => (
      <WorkbenchComposer
        value="发送当前空间消息"
        mode={DEFAULT_WORKBENCH_MODE}
        loading={false}
        spaceId={spaceId}
        onValueChange={vi.fn()}
        onModeChange={vi.fn()}
        onSubmit={onSubmit}
      />
    );

    await act(async () => {
      root.render(renderComposer('space-a'));
      await Promise.resolve();
    });
    onSubmit.mockClear();

    const getSendButton = () =>
      Array.from(container.querySelectorAll<HTMLButtonElement>('button')).find(
        button => button.textContent?.trim() === '发送',
      )!;
    act(() => {
      flushSync(() => root.render(renderComposer('space-b')));
      getSendButton().click();
    });
    expect(onSubmit).not.toHaveBeenCalled();

    await act(async () => {
      await Promise.resolve();
    });
    act(() => getSendButton().click());
    expect(onSubmit).toHaveBeenCalledTimes(1);
    expect(
      onSubmit.mock.calls[0][0].runtimeSettings.skills.allowed_skills,
    ).toEqual(['persistent-b']);
  });
});
