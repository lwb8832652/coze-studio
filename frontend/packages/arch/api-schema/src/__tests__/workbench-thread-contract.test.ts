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

import { readFileSync } from 'node:fs';

import { describe, expect, it } from 'vitest';

import * as threadSchema from '../idl/workbench/thread';

const generatedSource = readFileSync(
  new URL('../idl/workbench/thread.ts', import.meta.url),
  'utf8',
);

const canonicalAPIFunctions = [
  'CreateCanonicalThread',
  'SearchCanonicalThreads',
  'GetCanonicalThread',
  'PatchCanonicalThread',
  'DeleteCanonicalThread',
  'GetCanonicalThreadState',
  'UpdateCanonicalThreadState',
  'GetCanonicalThreadHistory',
  'PostCanonicalThreadHistory',
  'ListCanonicalThreadMessages',
  'ListCanonicalRuns',
  'CreateCanonicalRun',
  'StreamCanonicalRun',
  'WaitCanonicalRun',
  'GetCanonicalRun',
  'ReconnectCanonicalRunStream',
  'JoinCanonicalRun',
  'CancelCanonicalRun',
  'ResumeCanonicalRun',
  'ListCanonicalRunEvents',
  'ListCanonicalRunMessages',
] as const;

function interfaceSource(name: string): string {
  const match = generatedSource.match(
    new RegExp(`export interface ${name} \\{([\\s\\S]*?)\\n\\}`),
  );

  expect(match, `${name} must be generated`).not.toBeNull();
  return match?.[1] ?? '';
}

describe('canonical Workbench thread generated contract', () => {
  it('keeps public thread and run IDs as TypeScript strings', () => {
    expect(interfaceSource('CanonicalThread')).toMatch(
      /thread_id:\s*string[,;]/,
    );
    expect(interfaceSource('CanonicalRun')).toMatch(/thread_id:\s*string[,;]/);
    expect(interfaceSource('CanonicalRun')).toMatch(/run_id:\s*string[,;]/);
  });

  it('exports exactly the 21 canonical createAPI functions', () => {
    const generatedAPIFunctions = Array.from(
      generatedSource.matchAll(
        /export const (\w+) = \/\*#__PURE__\*\/createAPI</g,
      ),
      match => match[1],
    );

    expect(generatedAPIFunctions).toEqual(canonicalAPIFunctions);
    for (const functionName of canonicalAPIFunctions) {
      expect(threadSchema[functionName]).toBeTypeOf('function');
    }
  });

  it('generates the canonical route metadata without forbidden POST variants', () => {
    expect(generatedSource).toContain('"url": "/api/workbench/threads"');
    expect(generatedSource).toContain('"method": "PATCH"');
    expect(generatedSource).not.toMatch(
      /"url": "\/api\/workbench\/threads\/:thread_id\/runs\/:run_id\/stream"[\s\S]{0,100}"method": "POST"/,
    );
    expect(generatedSource).not.toMatch(
      /"url": "\/api\/workbench\/threads\/:thread_id\/runs\/:run_id\/join"[\s\S]{0,100}"method": "POST"/,
    );
  });
});
