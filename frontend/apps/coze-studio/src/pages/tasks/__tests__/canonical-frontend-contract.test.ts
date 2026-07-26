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

import { join, resolve } from 'node:path';
import { readFileSync, readdirSync } from 'node:fs';

import { describe, expect, it } from 'vitest';

const getProductionSourceFiles = (root: string): string[] =>
  readdirSync(root, { withFileTypes: true }).flatMap(entry => {
    const path = join(root, entry.name);

    if (entry.isDirectory()) {
      return entry.name === '__tests__' ? [] : getProductionSourceFiles(path);
    }

    return /\.tsx?$/.test(entry.name) ? [path] : [];
  });

const productionFiles = [
  ...getProductionSourceFiles(resolve(__dirname, '..')),
  ...getProductionSourceFiles(resolve(__dirname, '../../workbench')),
  ...getProductionSourceFiles(
    resolve(__dirname, '../../../components/workspace-sub-menu'),
  ),
  resolve(__dirname, '../../chats/task-thread-routes.ts'),
  resolve(__dirname, '../../../routes/index.tsx'),
];

const forbiddenIdentifiers = [
  'WorkbenchChat',
  'GetTask',
  'ListTasks',
  'CancelTask',
  'RetryTask',
  'ListTaskEvents',
  'getTask',
  'listTasks',
  'cancelTask',
  'retryTask',
  'listTaskEvents',
  'sendWorkbenchChat',
  'buildLegacyTaskDetailPath',
  'TaskEventDisplay',
  'getTaskEventDisplay',
  'getTaskEventText',
  'getTaskEventRunID',
  'mapTaskThreadRunEventToTaskEvent',
  'mapTaskThreadRunJournalMessageToTaskEvent',
  'mergeJournalTaskEvents',
  'mergeTaskEvents',
  'parseTaskEventPayload',
] as const;

const forbiddenPatterns = [
  new RegExp(`\\b(?:${forbiddenIdentifiers.join('|')})\\b`, 'g'),
  /\bworkbenchTask\.(?:ChatTask|TaskStatus|TaskEvent)\b/g,
  /\blegacy_task_id\b/g,
  /\bsource_task_id\b/g,
  /tasks\/:task_id\b/g,
];

describe('canonical Workbench task frontend contract', () => {
  it('keeps production task and workbench sources free of retired contracts', () => {
    const findings = productionFiles.flatMap(file => {
      const source = readFileSync(file, 'utf8');

      return forbiddenPatterns.flatMap(pattern =>
        Array.from(source.matchAll(pattern), match => ({
          file: file.replace(`${resolve(__dirname, '../../../../..')}/`, ''),
          symbol: match[0],
        })),
      );
    });

    expect(findings).toEqual([]);
  });
});
