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

/* eslint-disable @typescript-eslint/naming-convention -- Test doubles preserve external PascalCase component exports. */

import {
  type ButtonHTMLAttributes,
  type ReactElement,
  type ReactNode,
  type TextareaHTMLAttributes,
} from 'react';

import { afterEach, describe, expect, it, vi } from 'vitest';
import { act, Simulate } from 'react-dom/test-utils';
import { createRoot, type Root } from 'react-dom/client';

(
  globalThis as { IS_REACT_ACT_ENVIRONMENT?: boolean }
).IS_REACT_ACT_ENVIRONMENT = true;

vi.mock('@coze-arch/coze-design', () => ({
  Button: ({
    children,
    icon,
    loading,
    ...props
  }: ButtonHTMLAttributes<HTMLButtonElement> & {
    icon?: ReactNode;
    loading?: boolean;
  }) => (
    <button {...props} aria-busy={loading || undefined}>
      {icon}
      {children}
    </button>
  ),
  TextArea: ({
    autosize: _autosize,
    onChange,
    ...props
  }: Omit<TextareaHTMLAttributes<HTMLTextAreaElement>, 'onChange'> & {
    autosize?: boolean;
    onChange?: (value: string) => void;
  }) => (
    <textarea {...props} onChange={event => onChange?.(event.target.value)} />
  ),
}));

vi.mock('@coze-arch/coze-design/icons', () => ({
  IconCozSendFill: () => <span data-testid="send-icon" />,
  IconCozStopCircle: () => <span data-testid="stop-icon" />,
}));

import { ChatComposer } from '..';

const mountedRoots: Array<{ container: HTMLDivElement; root: Root }> = [];

const renderComposer = (element: ReactElement) => {
  const container = document.createElement('div');
  document.body.appendChild(container);
  const root = createRoot(container);
  mountedRoots.push({ container, root });

  act(() => {
    root.render(element);
  });

  return { container, root };
};

afterEach(() => {
  mountedRoots.splice(0).forEach(({ container, root }) => {
    act(() => {
      root.unmount();
    });
    container.remove();
  });
  vi.clearAllMocks();
});

describe('ChatComposer', () => {
  it('uses one accessible structure for hero and docked variants', () => {
    const { container } = renderComposer(
      <>
        <ChatComposer
          variant="hero"
          value=""
          placeholder="首页输入"
          onChange={vi.fn()}
          onSubmit={vi.fn()}
        />
        <ChatComposer
          variant="docked"
          value=""
          placeholder="详情输入"
          onChange={vi.fn()}
          onSubmit={vi.fn()}
        />
      </>,
    );

    const composers = Array.from(
      container.querySelectorAll('[aria-label="消息输入"]'),
    );
    expect(composers).toHaveLength(2);
    expect(composers[0]?.getAttribute('data-variant')).toBe('hero');
    expect(composers[1]?.getAttribute('data-variant')).toBe('docked');
    expect(
      composers.every(composer => composer.getAttribute('role') === 'group'),
    ).toBe(true);
    expect(
      composers.map(composer =>
        composer
          .querySelector('textarea[aria-label="消息"]')
          ?.getAttribute('placeholder'),
      ),
    ).toEqual(['首页输入', '详情输入']);
  });

  it('reports controlled input changes and submits the current value', () => {
    const onChange = vi.fn();
    const onSubmit = vi.fn();
    const { container } = renderComposer(
      <ChatComposer
        variant="hero"
        value="创建一份项目周报"
        onChange={onChange}
        onSubmit={onSubmit}
      />,
    );
    const textarea = container.querySelector('textarea') as HTMLTextAreaElement;

    act(() => {
      Simulate.change(textarea, {
        target: { value: '创建一份项目月报' },
      } as unknown as Event);
    });
    act(() => {
      (
        container.querySelector(
          'button[aria-label="发送消息"]',
        ) as HTMLButtonElement
      ).click();
    });

    expect(onChange).toHaveBeenCalledWith('创建一份项目月报');
    expect(onSubmit).toHaveBeenCalledWith('创建一份项目周报');
  });

  it('submits with Enter while preserving multiline and IME input', () => {
    const onSubmit = vi.fn();
    const { container } = renderComposer(
      <ChatComposer
        variant="docked"
        value="继续完善"
        onChange={vi.fn()}
        onSubmit={onSubmit}
      />,
    );
    const textarea = container.querySelector('textarea') as HTMLTextAreaElement;

    act(() => {
      Simulate.keyDown(textarea, { key: 'Enter', shiftKey: true });
    });
    expect(onSubmit).not.toHaveBeenCalled();

    act(() => {
      Simulate.compositionStart(textarea);
      Simulate.keyDown(textarea, { key: 'Enter' });
    });
    expect(onSubmit).not.toHaveBeenCalled();

    act(() => {
      Simulate.compositionEnd(textarea);
      Simulate.keyDown(textarea, { key: 'Enter' });
    });
    expect(onSubmit).toHaveBeenCalledOnce();
    expect(onSubmit).toHaveBeenCalledWith('继续完善');
  });

  it('exposes disabled and submitting states without accepting submissions', () => {
    const onSubmit = vi.fn();
    const { container, root } = renderComposer(
      <ChatComposer
        variant="hero"
        value="不可发送"
        disabled
        onChange={vi.fn()}
        onSubmit={onSubmit}
      />,
    );
    const disabledTextarea = container.querySelector(
      'textarea',
    ) as HTMLTextAreaElement;
    const disabledButton = container.querySelector(
      'button[aria-label="发送消息"]',
    ) as HTMLButtonElement;

    expect(disabledTextarea.disabled).toBe(true);
    expect(disabledButton.disabled).toBe(true);
    act(() => {
      Simulate.keyDown(disabledTextarea, { key: 'Enter' });
      disabledButton.click();
    });
    expect(onSubmit).not.toHaveBeenCalled();

    act(() => {
      root.render(
        <ChatComposer
          variant="hero"
          value="正在发送"
          submitting
          onChange={vi.fn()}
          onSubmit={onSubmit}
        />,
      );
    });

    expect(
      container
        .querySelector('[aria-label="消息输入"]')
        ?.getAttribute('aria-busy'),
    ).toBe('true');
    expect(
      (
        container.querySelector(
          'button[aria-label="发送消息"]',
        ) as HTMLButtonElement
      ).disabled,
    ).toBe(true);
  });

  it('switches the primary action to stop while streaming', () => {
    const onStop = vi.fn();
    const { container, root } = renderComposer(
      <ChatComposer
        variant="docked"
        value=""
        streaming
        onChange={vi.fn()}
        onSubmit={vi.fn()}
        onStop={onStop}
      />,
    );
    const stopButton = container.querySelector(
      'button[aria-label="停止生成"]',
    ) as HTMLButtonElement;

    expect(stopButton.disabled).toBe(false);
    act(() => {
      stopButton.click();
    });
    expect(onStop).toHaveBeenCalledOnce();

    act(() => {
      root.render(
        <ChatComposer
          variant="docked"
          value=""
          streaming
          stopping
          onChange={vi.fn()}
          onSubmit={vi.fn()}
          onStop={onStop}
        />,
      );
    });
    expect(
      (
        container.querySelector(
          'button[aria-label="停止生成"]',
        ) as HTMLButtonElement
      ).disabled,
    ).toBe(true);
  });

  it('keeps attachments, controls, footer slots, and custom editors composable', () => {
    const onSubmit = vi.fn();
    const { container } = renderComposer(
      <ChatComposer
        variant="hero"
        value="带上下文发送"
        onChange={vi.fn()}
        onSubmit={onSubmit}
        attachments={<span>附件.csv</span>}
        footerStart={<span>Pro</span>}
        skillControl={<button type="button">技能</button>}
        toolControl={<button type="button">拓展</button>}
        modelControl={<button type="button">模型</button>}
        footerEnd={<button type="button">@</button>}
        renderEditor={editorProps => (
          <textarea
            aria-label={editorProps['aria-label']}
            value={editorProps.value}
            onChange={event => editorProps.onChange(event.currentTarget.value)}
            onKeyDown={editorProps.onKeyDown}
          />
        )}
      />,
    );

    expect(container.textContent).toContain('附件.csv');
    expect(container.textContent).toContain('Pro');
    expect(container.textContent).toContain('技能');
    expect(container.textContent).toContain('拓展');
    expect(container.textContent).toContain('模型');
    expect(container.textContent).toContain('@');

    act(() => {
      Simulate.keyDown(container.querySelector('textarea') as Element, {
        key: 'Enter',
      });
    });
    expect(onSubmit).toHaveBeenCalledWith('带上下文发送');
  });
});
