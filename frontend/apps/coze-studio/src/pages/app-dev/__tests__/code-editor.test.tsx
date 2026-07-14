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

/* eslint-disable @typescript-eslint/naming-convention -- Mocked package exports preserve public component names. */

import { type ReactNode } from 'react';

import { afterEach, describe, expect, it, vi } from 'vitest';
import { act } from 'react-dom/test-utils';
import { createRoot, type Root } from 'react-dom/client';

import { CodeEditor } from '../components/code-editor';

interface MockEditorAction {
  id: string;
  label: string;
  keybindings: number[];
  run: () => void;
}

interface MockEditorProps {
  language?: string;
  loading?: ReactNode;
  onChange?: (value?: string) => void;
  onMount?: (
    editor: { addAction: (action: MockEditorAction) => void },
    monaco: {
      KeyCode: { KeyS: number };
      KeyMod: { CtrlCmd: number };
    },
  ) => void;
  options?: { readOnly?: boolean };
  path?: string;
  value?: string;
}

const editorState = vi.hoisted(() => ({
  props: undefined as MockEditorProps | undefined,
}));

vi.mock('@coze-arch/bot-monaco-editor', () => ({
  Editor: (props: MockEditorProps) => {
    editorState.props = props;
    return (
      <div
        data-language={props.language}
        data-path={props.path}
        data-read-only={String(Boolean(props.options?.readOnly))}
        data-testid="monaco-editor"
      />
    );
  },
}));

globalThis.IS_REACT_ACT_ENVIRONMENT = true;

describe('AppDev code editor', () => {
  let container: HTMLDivElement | undefined;
  let root: Root | undefined;

  afterEach(() => {
    if (root) {
      act(() => root?.unmount());
    }
    root = undefined;
    container = undefined;
    editorState.props = undefined;
    vi.clearAllMocks();
  });

  const renderEditor = ({
    onChange = vi.fn(),
    onSave = vi.fn(),
    saving = false,
  }: {
    onChange?: (value: string) => void;
    onSave?: () => void;
    saving?: boolean;
  } = {}) => {
    container ??= document.createElement('div');
    root ??= createRoot(container);

    act(() => {
      root?.render(
        <CodeEditor
          dirty
          onChange={onChange}
          onSave={onSave}
          path="src/App.tsx"
          saving={saving}
          value={'const title = "App";\nexport default title;'}
        />,
      );
    });

    return { onChange, onSave };
  };

  it('binds the selected file to Monaco and forwards edits', () => {
    const onChange = vi.fn();
    renderEditor({ onChange });

    const editor = container?.querySelector('[data-testid="monaco-editor"]');
    expect(editor?.getAttribute('data-path')).toBe('src/App.tsx');
    expect(editor?.getAttribute('data-language')).toBe('typescript');
    expect(container?.textContent).toContain('2 行');
    expect(container?.textContent).toContain('未保存');

    act(() =>
      editorState.props?.onChange?.('export default function App() {}'),
    );
    expect(onChange).toHaveBeenCalledWith('export default function App() {}');
  });

  it('registers a save action and blocks it while saving', () => {
    const onSave = vi.fn();
    renderEditor({ onSave });

    let action: MockEditorAction | undefined;
    act(() => {
      editorState.props?.onMount?.(
        {
          addAction: nextAction => {
            action = nextAction;
          },
        },
        { KeyCode: { KeyS: 49 }, KeyMod: { CtrlCmd: 2048 } },
      );
    });

    expect(action?.id).toBe('app-dev.save-current-file');
    act(() => action?.run());
    expect(onSave).toHaveBeenCalledTimes(1);

    renderEditor({ onSave, saving: true });
    expect(editorState.props?.options?.readOnly).toBe(true);
    act(() => action?.run());
    expect(onSave).toHaveBeenCalledTimes(1);
  });
});
