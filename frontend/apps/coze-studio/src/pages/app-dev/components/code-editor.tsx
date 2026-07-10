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

import { type KeyboardEvent, useEffect } from 'react';

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
  const handleEditorKeyDown = (event: KeyboardEvent<HTMLTextAreaElement>) => {
    if (event.key !== 'Tab') {
      return;
    }

    event.preventDefault();
    const target = event.currentTarget;
    const start = target.selectionStart;
    const end = target.selectionEnd;
    const nextValue = `${value.slice(0, start)}  ${value.slice(end)}`;
    onChange(nextValue);
    window.requestAnimationFrame(() => {
      target.selectionStart = start + 2;
      target.selectionEnd = start + 2;
    });
  };

  useEffect(() => {
    const handleKeydown = (event: KeyboardEvent) => {
      if ((event.metaKey || event.ctrlKey) && event.key.toLowerCase() === 's') {
        const target = event.target as HTMLElement | null;
        if (!target?.closest?.('.app-dev-code-editor')) {
          return;
        }
        event.preventDefault();
        onSave();
      }
    };

    window.addEventListener('keydown', handleKeydown);
    return () => window.removeEventListener('keydown', handleKeydown);
  }, [onSave]);

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
            <textarea
              key={path}
              value={value}
              spellCheck={false}
              aria-label="网页应用代码编辑器"
              readOnly={saving}
              onChange={event => onChange(event.target.value)}
              onKeyDown={handleEditorKeyDown}
            />
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
