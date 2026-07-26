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

interface CanonicalAPIExpectation {
  name: string;
  url: string;
  method: string;
  reqMapping?: Record<string, string[]>;
}

const canonicalAPIConfigs: CanonicalAPIExpectation[] = [
  {
    name: 'CreateCanonicalThread',
    url: '/api/workbench/threads',
    method: 'POST',
    reqMapping: {
      body: ['thread_id', 'metadata', 'if_exists', 'ttl', 'supersteps', 'coze'],
    },
  },
  {
    name: 'SearchCanonicalThreads',
    url: '/api/workbench/threads/search',
    method: 'POST',
    reqMapping: {
      body: [
        'metadata',
        'status',
        'ids',
        'limit',
        'offset',
        'sort_by',
        'sort_order',
        'values',
        'select',
        'extract',
      ],
    },
  },
  {
    name: 'GetCanonicalThread',
    url: '/api/workbench/threads/:thread_id',
    method: 'GET',
    reqMapping: { path: ['thread_id'], query: ['include'] },
  },
  {
    name: 'PatchCanonicalThread',
    url: '/api/workbench/threads/:thread_id',
    method: 'PATCH',
    reqMapping: {
      path: ['thread_id'],
      header: ['Prefer'],
      body: ['metadata', 'ttl'],
    },
  },
  {
    name: 'DeleteCanonicalThread',
    url: '/api/workbench/threads/:thread_id',
    method: 'DELETE',
    reqMapping: { path: ['thread_id'] },
  },
  {
    name: 'GetCanonicalThreadState',
    url: '/api/workbench/threads/:thread_id/state',
    method: 'GET',
    reqMapping: {
      path: ['thread_id'],
      query: ['checkpoint', 'checkpoint_id', 'subgraphs'],
    },
  },
  {
    name: 'UpdateCanonicalThreadState',
    url: '/api/workbench/threads/:thread_id/state',
    method: 'POST',
    reqMapping: {
      path: ['thread_id'],
      body: ['values', 'as_node', 'checkpoint', 'checkpoint_id'],
    },
  },
  {
    name: 'GetCanonicalThreadHistory',
    url: '/api/workbench/threads/:thread_id/history',
    method: 'GET',
    reqMapping: {
      path: ['thread_id'],
      query: ['limit', 'before', 'checkpoint', 'checkpoint_id'],
    },
  },
  {
    name: 'PostCanonicalThreadHistory',
    url: '/api/workbench/threads/:thread_id/history',
    method: 'POST',
    reqMapping: {
      path: ['thread_id'],
      body: ['limit', 'before', 'checkpoint', 'checkpoint_id'],
    },
  },
  {
    name: 'ListCanonicalThreadMessages',
    url: '/api/workbench/threads/:thread_id/messages',
    method: 'GET',
    reqMapping: {
      path: ['thread_id'],
      query: ['before_seq', 'after_seq', 'limit'],
    },
  },
  {
    name: 'ListCanonicalRuns',
    url: '/api/workbench/threads/:thread_id/runs',
    method: 'GET',
    reqMapping: {
      path: ['thread_id'],
      query: ['status', 'limit', 'offset', 'parent_run_id', 'select'],
    },
  },
  {
    name: 'CreateCanonicalRun',
    url: '/api/workbench/threads/:thread_id/runs',
    method: 'POST',
  },
  {
    name: 'StreamCanonicalRun',
    url: '/api/workbench/threads/:thread_id/runs/stream',
    method: 'POST',
  },
  {
    name: 'WaitCanonicalRun',
    url: '/api/workbench/threads/:thread_id/runs/wait',
    method: 'POST',
  },
  {
    name: 'GetCanonicalRun',
    url: '/api/workbench/threads/:thread_id/runs/:run_id',
    method: 'GET',
    reqMapping: { path: ['thread_id', 'run_id'] },
  },
  {
    name: 'ReconnectCanonicalRunStream',
    url: '/api/workbench/threads/:thread_id/runs/:run_id/stream',
    method: 'GET',
    reqMapping: {
      path: ['thread_id', 'run_id'],
      query: ['after_event_id', 'cancel_on_disconnect', 'stream_mode'],
      header: ['Last-Event-ID'],
    },
  },
  {
    name: 'JoinCanonicalRun',
    url: '/api/workbench/threads/:thread_id/runs/:run_id/join',
    method: 'GET',
    reqMapping: {
      path: ['thread_id', 'run_id'],
      query: ['cancel_on_disconnect'],
    },
  },
  {
    name: 'CancelCanonicalRun',
    url: '/api/workbench/threads/:thread_id/runs/:run_id/cancel',
    method: 'POST',
    reqMapping: {
      path: ['thread_id', 'run_id'],
      query: ['action', 'wait'],
    },
  },
  {
    name: 'ResumeCanonicalRun',
    url: '/api/workbench/threads/:thread_id/runs/:run_id/resume',
    method: 'POST',
    reqMapping: {
      path: ['thread_id', 'run_id'],
      body: ['interrupt_id', 'response'],
    },
  },
  {
    name: 'ListCanonicalRunEvents',
    url: '/api/workbench/threads/:thread_id/runs/:run_id/events',
    method: 'GET',
    reqMapping: {
      path: ['thread_id', 'run_id'],
      query: ['after_event_id', 'event_types', 'limit'],
    },
  },
  {
    name: 'ListCanonicalRunMessages',
    url: '/api/workbench/threads/:thread_id/runs/:run_id/messages',
    method: 'GET',
    reqMapping: {
      path: ['thread_id', 'run_id'],
      query: ['before_seq', 'after_seq', 'limit'],
    },
  },
];

const canonicalRunBody = [
  'assistant_id',
  'input',
  'command',
  'metadata',
  'config',
  'context',
  'stream_mode',
  'multitask_strategy',
  'on_disconnect',
  'durability',
  'stream_resumable',
  'stream_subgraphs',
  'if_not_exists',
  'webhook',
  'on_completion',
  'after_seconds',
  'feedback_keys',
  'interrupt_before',
  'interrupt_after',
  'checkpoint',
  'checkpoint_id',
  'langsmith_tracer',
] as const;

function interfaceSource(name: string): string {
  const match = generatedSource.match(
    new RegExp(`export interface ${name} \\{([\\s\\S]*?)\\n\\}`),
  );

  expect(match, `${name} must be generated`).not.toBeNull();
  return match?.[1] ?? '';
}

function apiConfig(name: string): {
  url: string;
  method: string;
  name: string;
  reqMapping?: Record<string, string[]>;
} {
  const declarationStart = generatedSource.indexOf(`export const ${name} =`);
  expect(declarationStart, `${name} must be generated`).toBeGreaterThanOrEqual(
    0,
  );

  const objectStart = generatedSource.indexOf('>({', declarationStart) + 2;
  const objectEnd = generatedSource.indexOf('\n});', objectStart);
  expect(objectStart, `${name} config must start`).toBeGreaterThanOrEqual(2);
  expect(objectEnd, `${name} config must end`).toBeGreaterThan(objectStart);

  return JSON.parse(generatedSource.slice(objectStart, objectEnd + 2));
}

describe('canonical Workbench thread generated contract', () => {
  it('keeps public thread and run IDs as TypeScript strings', () => {
    expect(interfaceSource('CanonicalRouteRequest').trim()).toBe(
      'thread_id: string',
    );
    expect(interfaceSource('CanonicalRouteRequest')).not.toMatch(
      /thread_id\?:\s*string[,;]/,
    );
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

  it('freezes every canonical method, path, and request mapping', () => {
    const runMapping = {
      path: ['thread_id'],
      body: [...canonicalRunBody],
      header: ['Idempotency-Key'],
    };
    const waitMapping = {
      ...runMapping,
      body: [...canonicalRunBody, 'raise_error'],
    };

    const expectedConfigs = canonicalAPIConfigs.map(expected => ({
      ...expected,
      reqMapping:
        expected.name === 'CreateCanonicalRun' ||
        expected.name === 'StreamCanonicalRun'
          ? runMapping
          : expected.name === 'WaitCanonicalRun'
            ? waitMapping
            : expected.reqMapping,
    }));
    const actualConfigs = canonicalAPIFunctions.map(name => {
      const actual = apiConfig(name);
      return {
        name: actual.name,
        url: actual.url,
        method: actual.method,
        reqMapping: actual.reqMapping,
      };
    });

    expect(actualConfigs).toEqual(expectedConfigs);

    expect(apiConfig('StreamCanonicalRun').reqMapping).toEqual(
      apiConfig('CreateCanonicalRun').reqMapping,
    );
    expect(apiConfig('WaitCanonicalRun').reqMapping).toEqual(waitMapping);
    expect(generatedSource).not.toMatch(
      /"url": "\/api\/workbench\/threads\/:thread_id\/runs\/:run_id\/stream"[\s\S]{0,100}"method": "POST"/,
    );
    expect(generatedSource).not.toMatch(
      /"url": "\/api\/workbench\/threads\/:thread_id\/runs\/:run_id\/join"[\s\S]{0,100}"method": "POST"/,
    );
  });
});
