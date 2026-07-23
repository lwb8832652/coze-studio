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

import { type ReactElement } from 'react';

import { afterEach, describe, expect, it, vi } from 'vitest';
import { act, Simulate } from 'react-dom/test-utils';
import { createRoot, type Root } from 'react-dom/client';

vi.mock('@coze-arch/coze-design', () => ({
  Button: ({
    children,
    icon,
    loading: _loading,
    ...props
  }: React.ButtonHTMLAttributes<HTMLButtonElement> & {
    icon?: React.ReactNode;
    loading?: boolean;
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
  IconCozSendFill: () => <span />,
  IconCozStopCircle: () => <span />,
}));

import { ChatComposer, type ChatComposerEditorProps } from '../index';

(
  globalThis as { IS_REACT_ACT_ENVIRONMENT?: boolean }
).IS_REACT_ACT_ENVIRONMENT = true;

const mountedRoots: Array<{ container: HTMLDivElement; root: Root }> = [];

const renderComposer = (element: ReactElement) => {
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
});

const renderForwardingEditor =
  (observedProps: (props: ChatComposerEditorProps) => void) =>
  (props: ChatComposerEditorProps) => {
    observedProps(props);
    return (
      <textarea
        aria-label="自定义输入框"
        value={props.value}
        disabled={props.disabled}
        readOnly={props.readOnly}
        onChange={event => props.onChange(event.target.value)}
        onKeyDown={props.onKeyDown}
        onCompositionStart={props.onCompositionStart}
        onCompositionEnd={props.onCompositionEnd}
      />
    );
  };

describe('ChatComposer custom editor contract', () => {
  it('passes the variant autosize contract to the default editor', () => {
    const hero = renderComposer(
      <ChatComposer
        variant="hero"
        value=""
        onChange={vi.fn()}
        onSubmit={vi.fn()}
      />,
    );
    const docked = renderComposer(
      <ChatComposer
        variant="docked"
        value=""
        onChange={vi.fn()}
        onSubmit={vi.fn()}
      />,
    );

    expect(
      hero.querySelector<HTMLTextAreaElement>('textarea')?.dataset
        .autosizeMinRows,
    ).toBe('3');
    expect(
      hero.querySelector<HTMLTextAreaElement>('textarea')?.dataset
        .autosizeMaxRows,
    ).toBe('8');
    expect(
      docked.querySelector<HTMLTextAreaElement>('textarea')?.dataset
        .autosizeMinRows,
    ).toBe('1');
    expect(
      docked.querySelector<HTMLTextAreaElement>('textarea')?.dataset
        .autosizeMaxRows,
    ).toBe('6');
  });

  it('forwards disabled and readOnly while submitting', () => {
    let editorProps: ChatComposerEditorProps | undefined;

    const container = renderComposer(
      <ChatComposer
        variant="docked"
        value="不会丢失的草稿"
        submitting
        readOnly
        onChange={vi.fn()}
        onSubmit={vi.fn()}
        renderEditor={renderForwardingEditor(props => {
          editorProps = props;
        })}
      />,
    );

    expect(editorProps?.disabled).toBe(true);
    expect(editorProps?.readOnly).toBe(true);
    expect(
      container.querySelector<HTMLTextAreaElement>(
        'textarea[aria-label="自定义输入框"]',
      )?.disabled,
    ).toBe(true);
  });

  it('forwards keyboard and composition handlers without submitting during IME', () => {
    const onSubmit = vi.fn();
    let editorProps: ChatComposerEditorProps | undefined;

    const container = renderComposer(
      <ChatComposer
        variant="hero"
        value="输入中的内容"
        onChange={vi.fn()}
        onSubmit={onSubmit}
        renderEditor={renderForwardingEditor(props => {
          editorProps = props;
        })}
      />,
    );

    expect(editorProps?.onKeyDown).toEqual(expect.any(Function));
    expect(editorProps?.onCompositionStart).toEqual(expect.any(Function));
    expect(editorProps?.onCompositionEnd).toEqual(expect.any(Function));

    const editor = container.querySelector<HTMLTextAreaElement>(
      'textarea[aria-label="自定义输入框"]',
    );
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
});
