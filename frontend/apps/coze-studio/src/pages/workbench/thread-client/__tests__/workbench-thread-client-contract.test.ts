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

import { basename, resolve } from 'node:path';
import { existsSync, readFileSync } from 'node:fs';

import { describe, expect, it } from 'vitest';

import {
  legacyTaskThreadReference,
  unwrapLegacyTaskThreadResponse,
} from './legacy-task-thread-reference';
import { pairedTransportFixtures } from './fixtures';

const productionFiles = [
  resolve(__dirname, '../types.ts'),
  resolve(__dirname, '../workbench-thread-client.ts'),
  resolve(__dirname, '../index.ts'),
];

const readProductionSource = (file: string) =>
  existsSync(file) ? readFileSync(file, 'utf8') : '';

const forbiddenPatterns = [
  /@coze-studio\/api-schema/g,
  /\bworkbench(?:Task|Thread)\b/g,
  /['"`]\/api\/[^'"`\s]*/g,
  /\btask_threads\b/g,
  /\bfetch\s*\(/g,
  /\bnew\s+(?:EventSource|FormData|Request|URL|URLSearchParams|WebSocket|XMLHttpRequest)\s*\(/g,
  /legacy-task-thread-reference/g,
] as const;

describe('WorkbenchThreadClient production boundary', () => {
  it('stays isolated from schemas, routes, and browser transports', () => {
    const findings = productionFiles.flatMap(file => {
      const source = readProductionSource(file);

      return forbiddenPatterns.flatMap(pattern =>
        Array.from(source.matchAll(pattern), match => ({
          file: basename(file),
          symbol: match[0],
        })),
      );
    });

    expect(findings).toEqual([]);
  });

  it('exposes canonical_v1 as its only public contract literal', () => {
    const clientSource = readProductionSource(productionFiles[1]);
    const contractLiterals = Array.from(
      clientSource.matchAll(/\breadonly\s+contract\s*:\s*(['"`])([^'"`]+)\1/g),
      match => match[2],
    );

    expect(contractLiterals).toEqual(['canonical_v1']);
  });

  it('keeps one frozen V1/canonical pair per visible resource family', () => {
    expect(Object.keys(pairedTransportFixtures)).toEqual([
      'thread',
      'todo',
      'message',
      'run',
      'run_event',
      'upload',
      'artifact',
      'artifact_scan_job',
      'token_usage',
      'memory',
      'memory_audit',
      'guardrail_audit',
      'mcp_runtime_audit',
      'human_interaction',
    ]);

    for (const fixture of Object.values(pairedTransportFixtures)) {
      expect(Object.isFrozen(fixture.v1)).toBe(true);
      expect(Object.isFrozen(fixture.canonical)).toBe(true);
      expect(Object.isFrozen(fixture.visible)).toBe(true);
      expect(JSON.stringify(fixture.canonical)).not.toContain('"space_id"');
    }
  });

  it('keeps the legacy envelope helper test-only and side-effect free', () => {
    expect(legacyTaskThreadReference.envelope).toBe('code_msg_data');
    expect(
      unwrapLegacyTaskThreadResponse({
        code: 0,
        msg: 'success',
        data: { thread_id: 'thread-1' },
      }),
    ).toEqual({ thread_id: 'thread-1' });
  });
});
