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

import { useLayoutEffect, useRef, useState } from 'react';

import { IconCozCopy } from '@coze-arch/coze-design/icons';
import { Button } from '@coze-arch/coze-design';

import { JournalViewState } from './view-state';
import type { JournalSnapshotViewProps } from './types';
import { runJournalSnapshotAction } from './snapshot-action';

const terminalOutput = ({ snapshot }: JournalSnapshotViewProps): string => {
  const terminal = snapshot?.content?.terminal;
  if (terminal?.output || terminal?.stdout || terminal?.stderr) {
    return (
      terminal.output ||
      [terminal.stdout, terminal.stderr].filter(Boolean).join('\n')
    );
  }
  return (
    snapshot?.fragments
      .filter(fragment =>
        ['terminal_stdout', 'terminal_stderr'].includes(fragment.kind ?? ''),
      )
      .map(fragment => fragment.content ?? '')
      .join('') ?? ''
  );
};

export const JournalTerminalView = (props: JournalSnapshotViewProps) => {
  const { snapshot, status, onSnapshotAction } = props;
  const [activeAction, setActiveAction] = useState('');
  const surfaceRef = useRef<HTMLDivElement>(null);
  const followOutputRef = useRef(true);
  const terminal = snapshot?.content?.terminal;
  const output = terminalOutput(props);
  const canRender = status === 'ready' || status === 'streaming';

  useLayoutEffect(() => {
    const surface = surfaceRef.current;
    if (!canRender || !surface || !followOutputRef.current) {
      return;
    }
    surface.scrollTop = surface.scrollHeight;
  }, [canRender, output, snapshot?.snapshot_id]);

  if (!canRender || !snapshot || !terminal) {
    return <JournalViewState status={status} />;
  }

  const runAction = (action: 'copy_command' | 'copy_output') => {
    if (!onSnapshotAction || activeAction) {
      return;
    }
    setActiveAction(action);
    void runJournalSnapshotAction(
      () => onSnapshotAction(action),
      () => setActiveAction(''),
    );
  };

  return (
    <div className="journal-terminal-view">
      <header>
        <div>
          <strong>{terminal.working_directory || '终端会话'}</strong>
          <span>
            {terminal.duration_ms !== undefined
              ? `${terminal.duration_ms} ms`
              : '执行快照'}
          </span>
        </div>
        <div>
          <Button
            aria-label="复制命令"
            disabled={!onSnapshotAction || Boolean(activeAction)}
            icon={<IconCozCopy />}
            loading={activeAction === 'copy_command'}
            size="small"
            theme="borderless"
            onClick={() => runAction('copy_command')}
          />
          <Button
            aria-label="复制终端输出"
            disabled={!onSnapshotAction || Boolean(activeAction)}
            icon={<IconCozCopy />}
            loading={activeAction === 'copy_output'}
            size="small"
            theme="borderless"
            onClick={() => runAction('copy_output')}
          />
        </div>
      </header>
      <div
        aria-busy={status === 'streaming'}
        className="journal-terminal-surface"
        data-journal-scroll-key="terminal-surface"
        ref={surfaceRef}
        onScroll={event => {
          const surface = event.currentTarget;
          followOutputRef.current =
            surface.scrollHeight - surface.scrollTop - surface.clientHeight <=
            24;
        }}
      >
        <pre className="journal-terminal-command">
          <code>{`$ ${terminal.command}`}</code>
        </pre>
        <pre className="journal-terminal-output">
          <code>{output}</code>
        </pre>
        <footer data-exit-code={terminal.exit_code ?? 0}>
          退出码 {terminal.exit_code ?? 0}
        </footer>
      </div>
    </div>
  );
};
