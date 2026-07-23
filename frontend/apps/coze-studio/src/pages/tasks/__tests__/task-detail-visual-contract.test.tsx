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

  it('keeps user metadata inside the bubble and uses library icons for todos', () => {
    const conversation = taskSource('conversation-turn.tsx');
    const todos = taskSource('task-execution-todo-dock.tsx');

    expect(conversation).toMatch(
      /coze-prototype-user-bubble[\s\S]*?coze-prototype-turn-time/,
    );
    expect(conversation).toContain('IconCozCheckMark');
    expect(todos).toContain('IconCozListDisorder');
    expect(todos).toContain('IconCozArrowDown');
    expect(todos).not.toContain('☷');
    expect(todos).not.toMatch(/>\s*\^\s*</);
  });

  it('uses the compact execution summary copy from the approved target', () => {
    const summary = taskSource('execution-summary.tsx');

    expect(summary).toContain('可用技能目录');
    expect(summary).not.toContain('查看执行详情');
  });
});
