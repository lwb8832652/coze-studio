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

const threadSurfaceFiles = [
  ...getProductionSourceFiles(resolve(__dirname, '..')),
  ...getProductionSourceFiles(resolve(__dirname, '../../workbench')),
  resolve(
    __dirname,
    '../../../components/workspace-sub-menu/workspace-task-list.tsx',
  ),
];

const canonicalTransportFiles = new Set([
  resolve(
    __dirname,
    '../../workbench/thread-client/adapters/canonical-thread-adapter.ts',
  ),
  resolve(__dirname, '../../workbench/thread-client/canonical-fetch.ts'),
  resolve(
    __dirname,
    '../../workbench/thread-client/canonical-thread-client.ts',
  ),
]);

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

const forbiddenThreadGeneratedPatterns = [
  /\bworkbenchTask\.[A-Za-z0-9_]*TaskThread[A-Za-z0-9_]*\b/g,
  /\bworkbenchThread\b/g,
];

const forbiddenThreadTransportPatterns = [
  /\/api\/workbench\/task_threads\b/g,
  /\/api\/threads\b/g,
  /\/api\/runs\b/g,
  /\b(?:globalThis\.)?fetch\s*\(/g,
  /\bnew\s+EventSource\b/g,
];

const sourceFinding = (file: string, symbol: string) => ({
  file: file.replace(`${resolve(__dirname, '../../../../..')}/`, ''),
  symbol,
});

describe('canonical Workbench task frontend contract', () => {
  it('keeps production task and workbench sources free of retired contracts', () => {
    const findings = productionFiles.flatMap(file => {
      const source = readFileSync(file, 'utf8');

      return forbiddenPatterns.flatMap(pattern =>
        Array.from(source.matchAll(pattern), match =>
          sourceFinding(file, match[0]),
        ),
      );
    });

    expect(findings).toEqual([]);
  });

  it('keeps Thread page production files behind the canonical client boundary', () => {
    const taskCenterService = resolve(
      __dirname,
      '../../task-center/service.ts',
    );
    const generatedTypeFindings = [
      ...threadSurfaceFiles,
      taskCenterService,
    ].flatMap(file => {
      const source = readFileSync(file, 'utf8');

      return forbiddenThreadGeneratedPatterns.flatMap(pattern =>
        Array.from(source.matchAll(pattern), match =>
          sourceFinding(file, match[0]),
        ),
      );
    });
    const transportFindings = threadSurfaceFiles.flatMap(file => {
      if (canonicalTransportFiles.has(file)) {
        return [];
      }
      const source = readFileSync(file, 'utf8');

      return forbiddenThreadTransportPatterns.flatMap(pattern =>
        Array.from(source.matchAll(pattern), match =>
          sourceFinding(file, match[0]),
        ),
      );
    });

    expect({
      generatedTypeFindings,
      transportFindings,
    }).toEqual({
      generatedTypeFindings: [],
      transportFindings: [],
    });
  });
});
