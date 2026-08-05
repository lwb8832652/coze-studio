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

import { resolve } from 'node:path';
import { readFileSync } from 'node:fs';

import { describe, expect, it } from 'vitest';

const taskSource = (fileName: string) =>
  readFileSync(resolve(__dirname, '..', fileName), 'utf8');

describe('task detail approved visual contract', () => {
  it('uses the approved content width and floats secondary composer controls', () => {
    const styles = taskSource('newx-task-ui.less');
    const dockedComposerWidthPattern = [
      String.raw`\.coze-prototype-followup[\s\S]*?`,
      String.raw`\.chat-composer\[data-variant='docked'\]`,
      String.raw`[\s\S]*?width:\s*min\(calc\(100% - 64px\), 1040px\)`,
    ].join('');

    expect(styles).toMatch(
      /\.coze-prototype-conversation-column[\s\S]*?width:\s*min\(calc\(100% - 64px\), 1040px\)/,
    );
    expect(styles).toMatch(new RegExp(dockedComposerWidthPattern));
    expect(styles).toMatch(
      /\.coze-prototype-followup[\s\S]*?\.chat-composer__footer-end[\s\S]*?position:\s*absolute/,
    );
    expect(styles).toMatch(
      /\.coze-prototype-todo-dock[\s\S]*?position:\s*absolute/,
    );
  });

  it('keeps the compact header hierarchy and creation timestamp', () => {
    const header = taskSource('task-detail-header.tsx');

    expect(header).toContain('创建于');
    expect(header).toContain('aria-label="返回全部任务"');
    expect(header).toContain('IconCozArrowDown');
    expect(header).not.toContain('IconCozArrowLeft');
  });

  it('reveals user metadata below the bubble on hover and uses library icons', () => {
    const conversation = taskSource('conversation-turn.tsx');
    const todos = taskSource('task-execution-todo-dock.tsx');
    const styles = taskSource('newx-task-ui.less');

    expect(conversation).toContain('coze-prototype-user-meta');
    expect(conversation).toContain('formatFullTimestamp');
    expect(conversation).toContain('aria-label="复制用户消息"');
    expect(conversation).toMatch(
      /coze-prototype-user-delivery[\s\S]*?<\/div>\s*<div className="coze-prototype-user-meta">/,
    );
    expect(styles).toMatch(
      /coze-prototype-user-meta[\s\S]*?max-height:\s*0[\s\S]*?visibility:\s*hidden[\s\S]*?opacity:\s*0/,
    );
    expect(styles).toMatch(
      /coze-prototype-user-turn:hover[\s\S]*?coze-prototype-user-meta[\s\S]*?visibility:\s*visible[\s\S]*?opacity:\s*1/,
    );
    expect(styles).toMatch(
      /\.coze-prototype-user-turn\s*\{[\s\S]*?position:\s*relative/,
    );
    expect(styles).toMatch(
      /\.coze-prototype-user-meta\s*\{[\s\S]*?position:\s*absolute/,
    );
    expect(conversation).toContain('IconCozCheckMark');
    expect(todos).toContain('IconCozListDisorder');
    expect(todos).toContain('IconCozArrowDown');
    expect(todos).not.toContain('☷');
    expect(todos).not.toMatch(/>\s*\^\s*</);
  });

  it('shows the current user identity above user messages', () => {
    const conversation = taskSource('conversation-turn.tsx');

    expect(conversation).toContain('useUserInfo');
    expect(conversation).toContain('coze-prototype-user-identity');
    expect(conversation).toContain('coze-prototype-user-avatar');
    expect(conversation).toContain('avatar_url');
    expect(conversation).toContain("userInfo?.screen_name || ''");
    expect(conversation).not.toContain('userInfo?.name ||');
  });

  it('uses the configured project identity in the approved compact Journal shell', () => {
    const conversation = taskSource('conversation-turn.tsx');
    const detail = taskSource('detail.tsx');
    const styles = taskSource('newx-task-ui.less');

    expect(conversation).toContain('journalMode = false');
    expect(conversation).toContain('<strong>{siteName}</strong>');
    expect(conversation).toContain('<WorkspaceMark variant="assistant" />');
    expect(conversation).not.toContain("journalMode ? 'Aime' : siteName");
    expect(detail).toContain('journalIntro = false');
    expect(detail).toContain('coze-prototype-journal-intro');
    expect(detail).toContain('data-journal-active={journalEvents.length > 0}');
    expect(detail).toContain('hideHeader={isJournalContinuation}');
    expect(detail).toContain('journalMode={isJournalRunMessage}');
    expect(detail).toContain('journalIntro={true}');
    expect(detail).toContain(
      'message={isJournalIntroMessage ? undefined : message}',
    );
    expect(detail).toContain(
      'isJournalIntroMessage && !hasCanonicalJournalIntro',
    );
    expect(styles).toMatch(
      /coze-prototype-assistant-turn-journal[\s\S]*?display:\s*block/,
    );
    expect(styles).not.toMatch(
      /data-journal-active='true'[\s\S]*?coze-prototype-user-turn[\s\S]*?display:\s*none/,
    );
  });

  it('uses the compact execution summary copy from the approved target', () => {
    const summary = taskSource('execution-summary.tsx');

    expect(summary).toContain('可用技能目录');
    expect(summary).not.toContain('查看执行详情');
  });

  it('keeps virtual Journal rows tall enough and action details readable', () => {
    const flow = taskSource('journal/journal-conversation-flow.tsx');
    const styles = taskSource('journal/journal.less');

    expect(flow).toContain('const actionRowHeight = 64;');
    expect(styles).toMatch(
      /\.journal-action-detail[\s\S]*?font-size:\s*10px[\s\S]*?color:\s*#59616d/,
    );
  });

  it('keeps Journal copy inside the conversation column when the detail panel is open', () => {
    const taskStyles = taskSource('newx-task-ui.less');
    const journalStyles = taskSource('journal/journal.less');

    expect(taskStyles).toMatch(
      /coze-prototype-assistant-turn-journal[\s\S]*?coze-prototype-assistant-turn-body[\s\S]*?grid-template-columns:\s*minmax\(0, 1fr\)/,
    );
    expect(journalStyles).toMatch(
      /\.journal-conversation-flow[\s\S]*?min-width:\s*0/,
    );
    expect(journalStyles).toMatch(
      /\.journal-execution-intro[\s\S]*?overflow-wrap:\s*anywhere/,
    );
  });
});
