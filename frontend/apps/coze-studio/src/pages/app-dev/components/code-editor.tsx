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

import { ErrorBoundary } from 'react-error-boundary';
import {
  type ComponentProps,
  useCallback,
  useEffect,
  useMemo,
  useRef,
} from 'react';

import { Editor } from '@coze-arch/bot-monaco-editor';

import { getLanguageFromPath } from '../utils/file-utils';

interface CodeEditorProps {
  path?: string;
  value: string;
  dirty?: boolean;
  loading?: boolean;
  saving?: boolean;
  onChange: (value: string) => void;
  onSave: () => void;
}

type MonacoEditorMount = NonNullable<ComponentProps<typeof Editor>['onMount']>;

export const CodeEditor = ({
  path,
  value,
  dirty,
  loading,
  saving,
  onChange,
  onSave,
}: CodeEditorProps) => {
  const language = path ? getLanguageFromPath(path) : 'plaintext';
  const lineCount = value ? value.split('\n').length : 1;
  const characterCount = value.length;
  const saveCommandRef = useRef({
    disabled: Boolean(loading || saving),
    onSave,
  });

  useEffect(() => {
    saveCommandRef.current = {
      disabled: Boolean(loading || saving),
      onSave,
    };
  }, [loading, onSave, saving]);

  const editorOptions = useMemo(
    () => ({
      ariaLabel: '网页应用代码编辑器',
      automaticLayout: true,
      bracketPairColorization: { enabled: true },
      cursorBlinking: 'smooth' as const,
      cursorSmoothCaretAnimation: 'on' as const,
      fixedOverflowWidgets: true,
      fontFamily:
        "'SFMono-Regular', Consolas, 'Liberation Mono', Menlo, monospace",
      fontLigatures: true,
      fontSize: 13,
      formatOnPaste: true,
      formatOnType: true,
      guides: {
        bracketPairs: true,
        indentation: true,
      },
      insertSpaces: true,
      lineHeight: 21,
      minimap: { enabled: false },
      padding: { bottom: 14, top: 14 },
      readOnly: Boolean(saving),
      renderLineHighlight: 'all' as const,
      renderWhitespace: 'selection' as const,
      scrollBeyondLastLine: false,
      smoothScrolling: true,
      stickyScroll: { enabled: true },
      tabSize: 2,
      wordWrap: 'off' as const,
    }),
    [saving],
  );

  const handleEditorMount = useCallback<MonacoEditorMount>((editor, monaco) => {
    editor.addAction({
      id: 'app-dev.save-current-file',
      label: '保存当前文件',
      keybindings: [monaco.KeyMod.CtrlCmd | monaco.KeyCode.KeyS],
      run: () => {
        const command = saveCommandRef.current;
        if (!command.disabled) {
          command.onSave();
        }
      },
    });
  }, []);

  if (!path) {
    return (
      <div className="app-dev-code-editor__empty">
        从左侧文件树选择一个文件开始编辑。
      </div>
    );
  }

  return (
    <div className="app-dev-code-editor">
      <header>
        <div>
          <strong>{path}</strong>
          <span>{language}</span>
          <small>
            {lineCount} 行 · {characterCount} 字符
          </small>
        </div>
        <div className="app-dev-code-editor__header-actions">
          <em data-dirty={dirty}>{dirty ? '未保存' : '已保存'}</em>
          <button type="button" onClick={onSave} disabled={loading || saving}>
            {saving ? '保存中...' : '保存'}
          </button>
        </div>
      </header>
      {loading ? (
        <div className="app-dev-code-editor__empty">正在加载文件内容...</div>
      ) : (
        <>
          <div className="app-dev-code-editor__surface">
            <ErrorBoundary
              resetKeys={[path]}
              fallbackRender={() => (
                <div
                  className="app-dev-code-editor__editor-state app-dev-code-editor__editor-state--error"
                  role="alert"
                >
                  代码编辑器加载失败，请刷新页面后重试。
                </div>
              )}
            >
              <Editor
                className="app-dev-code-editor__monaco"
                height="100%"
                keepCurrentModel={false}
                language={language}
                loading={
                  <div className="app-dev-code-editor__editor-state">
                    正在加载代码编辑器...
                  </div>
                }
                onChange={nextValue => onChange(nextValue ?? '')}
                onMount={handleEditorMount}
                options={editorOptions}
                path={path}
                saveViewState
                theme="vs-dark"
                value={value}
              />
            </ErrorBoundary>
          </div>
          <footer>
            <span>UTF-8</span>
            <span>Tab 插入两个空格</span>
            <span>⌘/Ctrl + S 保存</span>
          </footer>
        </>
      )}
    </div>
  );
};
