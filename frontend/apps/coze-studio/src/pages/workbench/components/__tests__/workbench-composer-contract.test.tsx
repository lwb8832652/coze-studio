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

import { type ComponentProps, type ReactElement, useState } from 'react';

import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import { act, Simulate } from 'react-dom/test-utils';
import { createRoot, type Root } from 'react-dom/client';

const mockListSkills = vi.hoisted(() => vi.fn());

vi.mock('../../../skill/service', () => ({
  listSkills: mockListSkills,
}));

vi.mock('@coze-arch/coze-design', () => ({
  Button: ({
    children,
    icon,
    loading: _loading,
    size: _size,
    color: _color,
    ...props
  }: React.ButtonHTMLAttributes<HTMLButtonElement> & {
    color?: string;
    icon?: React.ReactNode;
    loading?: boolean;
    size?: string;
  }) => (
    <button {...props}>
      {icon}
      {children}
    </button>
  ),
  TextArea: ({
    autosize,
    onChange,
    ...props
  }: Omit<React.TextareaHTMLAttributes<HTMLTextAreaElement>, 'onChange'> & {
    autosize?: boolean | { minRows: number; maxRows: number };
    onChange?: (value: string) => void;
  }) => (
    <textarea
      {...props}
      data-autosize-min-rows={
        typeof autosize === 'object' ? autosize.minRows : undefined
      }
      data-autosize-max-rows={
        typeof autosize === 'object' ? autosize.maxRows : undefined
      }
      onChange={event => onChange?.(event.target.value)}
    />
  ),
}));

vi.mock('@coze-arch/coze-design/icons', () => ({
  IconCozArrowDown: () => <span />,
  IconCozAt: () => <span aria-hidden="true" data-icon="at" />,
  IconCozArrowUp: () => <span />,
  IconCozArrowUpFill: () => <span />,
  IconCozCheckMark: () => <span />,
  IconCozCross: () => <span />,
  IconCozDiamondFill: () => <span />,
  IconCozLightbulb: () => <span />,
  IconCozLightbulbFill: () => <span />,
  IconCozLightningFill: () => <span />,
  IconCozLink: () => <span />,
  IconCozSendFill: () => <span />,
  IconCozRocketFill: () => <span />,
  IconCozSkill: () => <span />,
  IconCozStopCircle: () => <span />,
  IconCozUpload: () => <span />,
}));

vi.mock('../../extensions-popover', () => ({
  ExtensionsPopover: ({
    onChange,
    value,
  }: {
    onChange?: (selection: {
      enable_databases: string[];
      enable_kbs: string[];
      enable_mcp: string[];
      enable_skills: string[];
      explicit_enable_skills: string[];
    }) => void;
    value: {
      enable_databases: string[];
      enable_kbs: string[];
      enable_mcp: string[];
      enable_skills: string[];
      explicit_enable_skills: string[];
    };
  }) => (
    <button
      aria-label="选择持久扩展"
      type="button"
      onClick={() =>
        onChange?.({
          ...value,
          enable_skills: ['persistent-skill'],
          explicit_enable_skills: ['persistent-skill'],
        })
      }
    >
      拓展
    </button>
  ),
}));

vi.mock('../workbench-runtime-settings-control', () => ({
  WorkbenchRuntimeSettingsControl: () => null,
}));

vi.mock('../workbench-model-selector', () => ({
  findSelectedWorkbenchModel: () => undefined,
  getWorkbenchModelFallbackName: () => '测试模型',
  WorkbenchModelSelector: () => <button type="button">测试模型</button>,
}));

vi.mock('../workbench-composer-at-menu', () => {
  interface Selection {
    enable_databases: string[];
    enable_kbs: string[];
    enable_skills: string[];
    explicit_enable_skills: string[];
  }
  const addSelection = (
    selection: Selection,
    key: keyof Selection,
    id: string,
  ) => ({
    ...selection,
    [key]: Array.from(new Set([...selection[key], id])),
  });
  const removeSelection = (
    selection: Selection,
    key: keyof Selection,
    id: string,
  ) => ({
    ...selection,
    [key]: selection[key].filter(item => item !== id),
  });

  return {
    addDatabaseSelection: (selection: Selection, id: string) =>
      addSelection(selection, 'enable_databases', id),
    addKnowledgeSelection: (selection: Selection, id: string) =>
      addSelection(selection, 'enable_kbs', id),
    addSkillSelection: (selection: Selection, id: string) =>
      addSelection(
        addSelection(selection, 'enable_skills', id),
        'explicit_enable_skills',
        id,
      ),
    removeDatabaseSelection: (selection: Selection, id: string) =>
      removeSelection(selection, 'enable_databases', id),
    removeKnowledgeSelection: (selection: Selection, id: string) =>
      removeSelection(selection, 'enable_kbs', id),
    removeSkillSelection: (selection: Selection, id: string) =>
      removeSelection(
        removeSelection(selection, 'enable_skills', id),
        'explicit_enable_skills',
        id,
      ),
    getWorkbenchAtResourceIcon: () => 'S',
    AtMenu: ({
      onReferenceSelect,
    }: {
      onReferenceSelect: (reference: {
        id: string;
        name: string;
        resourceType: '技能';
      }) => void;
    }) => (
      <button
        type="button"
        aria-label="选择测试技能"
        onClick={() =>
          onReferenceSelect({
            id: 'transient-skill',
            name: '测试技能',
            resourceType: '技能',
          })
        }
      >
        测试技能
      </button>
    ),
  };
});

import { WorkbenchComposerBody } from '../workbench-composer-controls';
import {
  WorkbenchComposer,
  type WorkbenchComposerSubmitPayload,
} from '../workbench-composer';

(
  globalThis as { IS_REACT_ACT_ENVIRONMENT?: boolean }
).IS_REACT_ACT_ENVIRONMENT = true;

const mountedRoots: Array<{ container: HTMLDivElement; root: Root }> = [];

const renderElement = (element: ReactElement) => {
  const container = document.createElement('div');
  document.body.appendChild(container);
  const root = createRoot(container);
  mountedRoots.push({ container, root });
  act(() => {
    root.render(element);
  });
  return container;
};

afterEach(() => {
  mountedRoots.splice(0).forEach(({ container, root }) => {
    act(() => {
      root.unmount();
    });
    container.remove();
  });
  vi.clearAllMocks();
  window.localStorage.clear();
});

beforeEach(() => {
  mockListSkills.mockResolvedValue({
    data: {
      skills: [
        { id: 'skill-a', name: 'alpha', enabled: true },
        { id: 'skill-b', name: 'beta', enabled: true },
      ],
    },
  });
});

const baseProps = {
  mode: 'default',
  selectedModelId: 'test-model',
  selectedModelName: '测试模型',
  onModeChange: vi.fn(),
  onModelChange: vi.fn(),
  onStop: vi.fn(),
} as unknown as Partial<ComponentProps<typeof WorkbenchComposer>>;

const renderComposer = (
  props: Partial<ComponentProps<typeof WorkbenchComposer>> = {},
) =>
  renderElement(
    <WorkbenchComposer
      {...(baseProps as ComponentProps<typeof WorkbenchComposer>)}
      value="第一轮"
      onValueChange={vi.fn()}
      onSubmit={vi.fn()}
      {...props}
    />,
  );

describe('WorkbenchComposer interaction contract', () => {
  it('passes footerEnd through the public ChatComposer slot', () => {
    const container = renderComposer({
      footerEnd: <button type="button">Usage footer</button>,
    });

    expect(
      Array.from(container.querySelectorAll('button')).some(
        button => button.textContent === 'Usage footer',
      ),
    ).toBe(true);
  });

  it('disables the real rich input, attachment, and mention controls while loading', () => {
    const container = renderComposer({ loading: true });

    expect(
      container.querySelector<HTMLTextAreaElement>('textarea')?.disabled,
    ).toBe(true);
    expect(
      container.querySelector<HTMLButtonElement>('button[aria-label*="附件"]')
        ?.disabled,
    ).toBe(true);
    expect(
      container.querySelector<HTMLButtonElement>(
        'button[aria-label="添加上下文"]',
      )?.disabled,
    ).toBe(true);
    expect(
      container.querySelector<HTMLInputElement>('input[type="file"]')?.disabled,
    ).toBe(true);
  });

  it('does not submit the real rich input while an IME composition is active', () => {
    const onSubmit = vi.fn();
    const container = renderComposer({ onSubmit });

    const editor = container.querySelector<HTMLTextAreaElement>('textarea');
    expect(editor).not.toBeNull();
    act(() => {
      Simulate.compositionStart(editor as HTMLTextAreaElement);
      Simulate.keyDown(editor as HTMLTextAreaElement, {
        key: 'Enter',
        code: 'Enter',
      });
    });
    expect(onSubmit).not.toHaveBeenCalled();

    act(() => {
      Simulate.compositionEnd(editor as HTMLTextAreaElement);
      Simulate.keyDown(editor as HTMLTextAreaElement, {
        key: 'Enter',
        code: 'Enter',
      });
    });
    expect(onSubmit).toHaveBeenCalledTimes(1);
  });

  it('disables every editing entry while streaming but keeps stop available', () => {
    const onStop = vi.fn();
    const container = renderComposer({
      loading: false,
      stopMode: true,
      presentation: 'deerflow',
      onStop,
    });

    expect(
      container.querySelector<HTMLTextAreaElement>('textarea')?.disabled,
    ).toBe(true);
    expect(
      container.querySelector<HTMLButtonElement>(
        'button[aria-label="添加附件"]',
      )?.disabled,
    ).toBe(true);
    expect(
      container.querySelector<HTMLButtonElement>(
        'button[aria-label="添加上下文"]',
      )?.disabled,
    ).toBe(true);
    expect(
      container.querySelector<HTMLButtonElement>(
        'button[aria-label="选择模式"]',
      )?.disabled,
    ).toBe(true);
    const stopButton = container.querySelector<HTMLButtonElement>(
      'button[aria-label="停止任务"]',
    );
    expect(stopButton?.disabled).toBe(false);
    act(() => Simulate.click(stopButton as HTMLButtonElement));
    expect(onStop).toHaveBeenCalledTimes(1);
  });

  it('closes slash skill suggestions when streaming starts', async () => {
    const Harness = () => {
      const [streaming, setStreaming] = useState(false);
      const [value, setValue] = useState('/');

      return (
        <>
          <button
            aria-label="开始流式"
            type="button"
            onClick={() => setStreaming(true)}
          />
          <WorkbenchComposer
            {...(baseProps as ComponentProps<typeof WorkbenchComposer>)}
            loading={false}
            presentation="deerflow"
            spaceId="space-1"
            stopMode={streaming}
            value={value}
            onValueChange={setValue}
            onSubmit={vi.fn()}
          />
        </>
      );
    };
    const container = renderElement(<Harness />);
    const textarea = container.querySelector<HTMLTextAreaElement>('textarea');

    await act(async () => {
      Simulate.focus(textarea as HTMLTextAreaElement);
      await Promise.resolve();
      await Promise.resolve();
    });
    expect(
      container.querySelector('[aria-label="Skill suggestions"]'),
    ).not.toBeNull();

    act(() => {
      Simulate.click(
        container.querySelector<HTMLButtonElement>(
          'button[aria-label="开始流式"]',
        ) as HTMLButtonElement,
      );
    });

    expect(
      container.querySelector('[aria-label="Skill suggestions"]'),
    ).toBeNull();
  });

  it('guards every disabled slash option event', () => {
    const onApply = vi.fn();
    const onIndexChange = vi.fn();
    const onKeyDown = vi.fn();
    const container = renderElement(
      <WorkbenchComposerBody
        disabled
        mode="pro"
        presentation="deerflow"
        showSkillSuggestions
        skillSuggestionIndex={0}
        skillSuggestions={[
          { id: 'skill-a', name: 'alpha' },
          { id: 'skill-b', name: 'beta' },
        ]}
        value="/"
        onChange={vi.fn()}
        onSkillSuggestionApply={onApply}
        onSkillSuggestionIndexChange={onIndexChange}
        onSkillSuggestionKeyDown={onKeyDown}
      />,
    );
    const secondOption = container.querySelectorAll<HTMLButtonElement>(
      '.chat-workbench-skill-suggestion',
    )[1];
    const textarea = container.querySelector<HTMLTextAreaElement>('textarea');

    act(() => {
      Simulate.mouseEnter(secondOption);
      Simulate.click(secondOption);
      Simulate.keyDown(textarea as HTMLTextAreaElement, {
        key: 'ArrowDown',
      });
    });

    expect(onIndexChange).not.toHaveBeenCalled();
    expect(onApply).not.toHaveBeenCalled();
    expect(onKeyDown).not.toHaveBeenCalled();
  });

  it('uses controlled autosize with denser task-detail rows', () => {
    const homeContainer = renderComposer({ variant: 'home' });
    const detailContainer = renderComposer({ variant: 'detail' });
    const homeEditor =
      homeContainer.querySelector<HTMLTextAreaElement>('textarea');
    const detailEditor =
      detailContainer.querySelector<HTMLTextAreaElement>('textarea');

    expect(homeEditor?.dataset.autosizeMinRows).toBe('3');
    expect(homeEditor?.dataset.autosizeMaxRows).toBe('8');
    expect(detailEditor?.dataset.autosizeMinRows).toBe('1');
    expect(detailEditor?.dataset.autosizeMaxRows).toBe('6');
    expect(homeEditor?.hasAttribute('rows')).toBe(false);
    expect(detailEditor?.hasAttribute('rows')).toBe(false);
  });

  it('disables legacy attachments up front and keeps canonical attachments available', () => {
    const legacyContainer = renderComposer({
      capabilities: {
        attachments: false,
        attachmentDisabledReason: '旧版任务暂不支持附件续聊',
      },
    });
    const canonicalContainer = renderComposer({
      capabilities: { attachments: true },
    });

    expect(
      legacyContainer.querySelector<HTMLInputElement>('input[type="file"]')
        ?.disabled,
    ).toBe(true);
    expect(
      legacyContainer.querySelector<HTMLButtonElement>(
        'button[aria-label="添加附件"]',
      )?.disabled,
    ).toBe(true);
    expect(legacyContainer.textContent).toContain('旧版任务暂不支持附件续聊');
    expect(
      canonicalContainer.querySelector<HTMLInputElement>('input[type="file"]')
        ?.disabled,
    ).toBe(false);
    expect(
      canonicalContainer.querySelector<HTMLButtonElement>(
        'button[aria-label="添加附件"]',
      )?.disabled,
    ).toBe(false);
  });

  it('removes an attachment before submit', () => {
    const onSubmit = vi.fn();
    const container = renderComposer({ onSubmit });
    const fileInput =
      container.querySelector<HTMLInputElement>('input[type="file"]');
    const file = new File(['draft'], 'draft.txt', { type: 'text/plain' });

    expect(fileInput).not.toBeNull();
    act(() => {
      Simulate.change(fileInput as HTMLInputElement, {
        target: { files: [file] } as unknown as EventTarget & HTMLInputElement,
      });
    });
    const removeButton = container.querySelector<HTMLButtonElement>(
      'button[aria-label="移除附件 draft.txt"]',
    );
    expect(removeButton).not.toBeNull();
    act(() => {
      Simulate.click(removeButton as HTMLButtonElement);
    });
    act(() => {
      Simulate.keyDown(
        container.querySelector('textarea') as HTMLTextAreaElement,
        {
          key: 'Enter',
          code: 'Enter',
        },
      );
    });

    expect(onSubmit).toHaveBeenCalledWith(
      expect.objectContaining({ files: [] }),
    );
  });

  it('resets internal draft resources after success so the second payload is clean', async () => {
    const payloads: WorkbenchComposerSubmitPayload[] = [];

    const Harness = (): ReactElement => {
      const [value, setValue] = useState('第一轮');
      const [resetKey, setResetKey] = useState(0);

      return (
        <WorkbenchComposer
          {...(baseProps as ComponentProps<typeof WorkbenchComposer>)}
          value={value}
          resetKey={resetKey}
          onValueChange={setValue}
          onSubmit={payload => {
            payloads.push(payload);
            setValue('');
            setResetKey(key => key + 1);
          }}
        />
      );
    };

    const container = renderElement(<Harness />);
    const fileInput =
      container.querySelector<HTMLInputElement>('input[type="file"]');
    const file = new File(['context'], 'context.txt', { type: 'text/plain' });
    expect(fileInput).not.toBeNull();

    act(() => {
      Simulate.click(
        container.querySelector<HTMLButtonElement>(
          'button[aria-label="选择持久扩展"]',
        ) as HTMLButtonElement,
      );
    });
    act(() => {
      Simulate.change(fileInput as HTMLInputElement, {
        target: { files: [file] } as unknown as EventTarget & HTMLInputElement,
      });
    });
    act(() => {
      Simulate.change(
        container.querySelector('textarea') as HTMLTextAreaElement,
        {
          target: { value: '第一轮@' },
        },
      );
    });
    const skillButton = container.querySelector<HTMLButtonElement>(
      'button[aria-label="选择测试技能"]',
    );
    expect(skillButton).not.toBeNull();
    act(() => {
      Simulate.click(skillButton as HTMLButtonElement);
    });
    act(() => {
      Simulate.keyDown(
        container.querySelector('textarea') as HTMLTextAreaElement,
        {
          key: 'Enter',
          code: 'Enter',
        },
      );
    });

    await act(async () => {
      await Promise.resolve();
    });
    expect(payloads).toHaveLength(1);
    expect(container.textContent).not.toContain('context.txt');
    expect(payloads[0]).toEqual(
      expect.objectContaining({
        files: [file],
        enable_skills: ['persistent-skill', 'transient-skill'],
        message: expect.stringContaining('@测试技能'),
      }),
    );

    act(() => {
      Simulate.change(
        container.querySelector('textarea') as HTMLTextAreaElement,
        {
          target: { value: '第二轮' },
        },
      );
    });
    act(() => {
      Simulate.keyDown(
        container.querySelector('textarea') as HTMLTextAreaElement,
        {
          key: 'Enter',
          code: 'Enter',
        },
      );
    });

    await act(async () => {
      await Promise.resolve();
    });
    expect(payloads).toHaveLength(2);
    expect(payloads[1]).toEqual(
      expect.objectContaining({
        message: '第二轮',
        files: [],
      }),
    );
    expect(payloads[1]?.enable_skills ?? []).toEqual(['persistent-skill']);
  });

  it('clears transient mentions on task switch without dropping persistent extensions', async () => {
    const payloads: WorkbenchComposerSubmitPayload[] = [];

    const Harness = () => {
      const [taskId, setTaskId] = useState('task-old');
      const [value, setValue] = useState('旧消息');

      return (
        <>
          <button
            aria-label="切换任务"
            type="button"
            onClick={() => {
              setTaskId('task-new');
              setValue('新任务消息');
            }}
          />
          <WorkbenchComposer
            {...(baseProps as ComponentProps<typeof WorkbenchComposer>)}
            taskId={taskId}
            value={value}
            onValueChange={setValue}
            onSubmit={payload => payloads.push(payload)}
          />
        </>
      );
    };

    const container = renderElement(<Harness />);
    act(() => {
      Simulate.click(
        container.querySelector<HTMLButtonElement>(
          'button[aria-label="选择持久扩展"]',
        ) as HTMLButtonElement,
      );
      Simulate.change(
        container.querySelector('textarea') as HTMLTextAreaElement,
        { target: { value: '旧消息@' } },
      );
    });
    act(() => {
      Simulate.click(
        container.querySelector<HTMLButtonElement>(
          'button[aria-label="选择测试技能"]',
        ) as HTMLButtonElement,
      );
      Simulate.click(
        container.querySelector<HTMLButtonElement>(
          'button[aria-label="切换任务"]',
        ) as HTMLButtonElement,
      );
    });
    act(() => {
      Simulate.keyDown(
        container.querySelector('textarea') as HTMLTextAreaElement,
        { key: 'Enter', code: 'Enter' },
      );
    });

    await act(async () => {
      await Promise.resolve();
    });
    expect(payloads).toHaveLength(1);
    expect(payloads[0]).toEqual(
      expect.objectContaining({
        message: '新任务消息',
        enable_skills: ['persistent-skill'],
      }),
    );
    expect(payloads[0]?.message).not.toContain('@测试技能');
  });

  it('renders the library mention icon in the visible toolbar', () => {
    const container = renderComposer();
    const mentionButton = container.querySelector<HTMLButtonElement>(
      'button[aria-label="添加上下文"]',
    );

    expect(mentionButton?.querySelector('[data-icon="at"]')).not.toBeNull();
    expect(mentionButton?.textContent).not.toContain('@');
  });
});
