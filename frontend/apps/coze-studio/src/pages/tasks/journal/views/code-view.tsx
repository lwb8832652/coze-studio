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

import { useMemo, useState } from 'react';

import { IconCozCopy } from '@coze-arch/coze-design/icons';
import { Button } from '@coze-arch/coze-design';
import { Editor } from '@coze-arch/bot-monaco-editor';

import { JournalViewState } from './view-state';
import type { JournalSnapshotViewProps } from './types';
import { runJournalSnapshotAction } from './snapshot-action';

const codeText = ({ snapshot }: JournalSnapshotViewProps): string => {
  const direct = snapshot?.content?.code?.content;
  if (direct) {
    return direct;
  }
  return (
    snapshot?.fragments
      .filter(fragment => fragment.kind === 'code_lines')
      .map(fragment => fragment.content ?? '')
      .join('') ?? ''
  );
};

export const JournalCodeView = (props: JournalSnapshotViewProps) => {
  const { snapshot, status, onSnapshotAction } = props;
  const [copying, setCopying] = useState(false);
  const code = snapshot?.content?.code;
  const content = codeText(props);
  const options = useMemo(
    () => ({
      ariaLabel: '只读代码快照',
      automaticLayout: true,
      fontSize: 12,
      lineHeight: 20,
      minimap: { enabled: false },
      padding: { bottom: 16, top: 16 },
      readOnly: true,
      renderLineHighlight: 'none' as const,
      scrollBeyondLastLine: false,
      stickyScroll: { enabled: true },
      wordWrap: 'off' as const,
    }),
    [],
  );

  if (status !== 'ready' || !snapshot || !code || !content) {
    return <JournalViewState status={status} />;
  }

  return (
    <div className="journal-code-view">
      <aside
        className="journal-code-tree"
        data-journal-scroll-key="code-tree"
        aria-label="文件"
      >
        <strong>文件</strong>
        <button type="button" aria-current="page">
          {code.file_path}
        </button>
      </aside>
      <section className="journal-code-panel">
        <header>
          <div>
            <strong>{code.file_path}</strong>
            <span>
              只读 · {code.language || 'plaintext'} · {code.revision}
            </span>
          </div>
          {onSnapshotAction ? (
            <Button
              aria-label={`复制代码 ${code.file_path}`}
              icon={<IconCozCopy />}
              loading={copying}
              size="small"
              theme="borderless"
              onClick={() => {
                setCopying(true);
                void runJournalSnapshotAction(
                  () => onSnapshotAction('copy_code'),
                  () => setCopying(false),
                );
              }}
            />
          ) : null}
        </header>
        <div className="journal-code-editor">
          <Editor
            height="100%"
            keepCurrentModel={false}
            language={code.language || 'plaintext'}
            options={options}
            path={`${code.repository}/${code.file_path}@${code.revision}`}
            saveViewState
            theme="vs-dark"
            value={content}
          />
        </div>
      </section>
    </div>
  );
};
