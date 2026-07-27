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

import * as api from '../idl/workbench/thread';

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

const productMethods = [
  'AppendCanonicalThreadMessage',
  'GenerateCanonicalThreadSuggestions',
  'ListCanonicalThreadUploads',
  'UploadCanonicalThreadFiles',
  'DeleteCanonicalThreadUpload',
  'ListCanonicalThreadArtifacts',
  'GetCanonicalThreadArtifactContent',
  'GetCanonicalThreadArtifactSignedURL',
  'DeleteCanonicalThreadArtifact',
  'RestoreCanonicalThreadArtifact',
  'ReviewCanonicalThreadArtifactScan',
  'ListCanonicalThreadArtifactScanJobs',
  'RetryCanonicalThreadArtifactScanJob',
  'GetCanonicalThreadTokenUsage',
  'ListCanonicalThreadMemories',
  'UpdateCanonicalThreadMemory',
  'DeleteCanonicalThreadMemory',
  'RestoreCanonicalThreadMemory',
  'ClearCanonicalThreadMemories',
  'ExportCanonicalThreadMemories',
  'ImportCanonicalThreadMemories',
  'ListCanonicalThreadMemoryAuditEvents',
  'ListCanonicalThreadGuardrailAuditEvents',
  'ExportCanonicalThreadGuardrailAuditEvents',
  'ListCanonicalThreadMCPRuntimeAuditEvents',
  'RetryCanonicalSubagentRun',
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
      header: ['X-Coze-Space-ID'],
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
      header: ['X-Coze-Space-ID'],
    },
  },
  {
    name: 'GetCanonicalThread',
    url: '/api/workbench/threads/:thread_id',
    method: 'GET',
    reqMapping: {
      path: ['thread_id'],
      query: ['include'],
      header: ['X-Coze-Space-ID'],
    },
  },
  {
    name: 'PatchCanonicalThread',
    url: '/api/workbench/threads/:thread_id',
    method: 'PATCH',
    reqMapping: {
      path: ['thread_id'],
      header: ['Prefer', 'X-Coze-Space-ID'],
      body: ['metadata', 'ttl'],
    },
  },
  {
    name: 'DeleteCanonicalThread',
    url: '/api/workbench/threads/:thread_id',
    method: 'DELETE',
    reqMapping: {
      path: ['thread_id'],
      header: ['X-Coze-Space-ID'],
    },
  },
  {
    name: 'GetCanonicalThreadState',
    url: '/api/workbench/threads/:thread_id/state',
    method: 'GET',
    reqMapping: {
      path: ['thread_id'],
      query: ['checkpoint', 'checkpoint_id', 'subgraphs'],
      header: ['X-Coze-Space-ID'],
    },
  },
  {
    name: 'UpdateCanonicalThreadState',
    url: '/api/workbench/threads/:thread_id/state',
    method: 'POST',
    reqMapping: {
      path: ['thread_id'],
      body: ['values', 'as_node', 'checkpoint', 'checkpoint_id'],
      header: ['X-Coze-Space-ID'],
    },
  },
  {
    name: 'GetCanonicalThreadHistory',
    url: '/api/workbench/threads/:thread_id/history',
    method: 'GET',
    reqMapping: {
      path: ['thread_id'],
      query: ['limit', 'before', 'checkpoint', 'checkpoint_id'],
      header: ['X-Coze-Space-ID'],
    },
  },
  {
    name: 'PostCanonicalThreadHistory',
    url: '/api/workbench/threads/:thread_id/history',
    method: 'POST',
    reqMapping: {
      path: ['thread_id'],
      body: ['limit', 'before', 'checkpoint', 'checkpoint_id'],
      header: ['X-Coze-Space-ID'],
    },
  },
  {
    name: 'ListCanonicalThreadMessages',
    url: '/api/workbench/threads/:thread_id/messages',
    method: 'GET',
    reqMapping: {
      path: ['thread_id'],
      query: ['before_seq', 'after_seq', 'limit'],
      header: ['X-Coze-Space-ID'],
    },
  },
  {
    name: 'ListCanonicalRuns',
    url: '/api/workbench/threads/:thread_id/runs',
    method: 'GET',
    reqMapping: {
      path: ['thread_id'],
      query: ['status', 'limit', 'offset', 'parent_run_id', 'select'],
      header: ['X-Coze-Space-ID'],
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
    reqMapping: {
      path: ['thread_id', 'run_id'],
      header: ['X-Coze-Space-ID'],
    },
  },
  {
    name: 'ReconnectCanonicalRunStream',
    url: '/api/workbench/threads/:thread_id/runs/:run_id/stream',
    method: 'GET',
    reqMapping: {
      path: ['thread_id', 'run_id'],
      query: ['after_event_id', 'cancel_on_disconnect', 'stream_mode'],
      header: ['Last-Event-ID', 'X-Coze-Space-ID'],
    },
  },
  {
    name: 'JoinCanonicalRun',
    url: '/api/workbench/threads/:thread_id/runs/:run_id/join',
    method: 'GET',
    reqMapping: {
      path: ['thread_id', 'run_id'],
      query: ['cancel_on_disconnect'],
      header: ['X-Coze-Space-ID'],
    },
  },
  {
    name: 'CancelCanonicalRun',
    url: '/api/workbench/threads/:thread_id/runs/:run_id/cancel',
    method: 'POST',
    reqMapping: {
      path: ['thread_id', 'run_id'],
      query: ['action', 'wait'],
      header: ['X-Coze-Space-ID'],
    },
  },
  {
    name: 'ResumeCanonicalRun',
    url: '/api/workbench/threads/:thread_id/runs/:run_id/resume',
    method: 'POST',
    reqMapping: {
      path: ['thread_id', 'run_id'],
      body: ['interrupt_id', 'response'],
      header: ['X-Coze-Space-ID'],
    },
  },
  {
    name: 'ListCanonicalRunEvents',
    url: '/api/workbench/threads/:thread_id/runs/:run_id/events',
    method: 'GET',
    reqMapping: {
      path: ['thread_id', 'run_id'],
      query: ['after_event_id', 'event_types', 'limit'],
      header: ['X-Coze-Space-ID'],
    },
  },
  {
    name: 'ListCanonicalRunMessages',
    url: '/api/workbench/threads/:thread_id/runs/:run_id/messages',
    method: 'GET',
    reqMapping: {
      path: ['thread_id', 'run_id'],
      query: ['before_seq', 'after_seq', 'limit'],
      header: ['X-Coze-Space-ID'],
    },
  },
];

const productAPIConfigs: CanonicalAPIExpectation[] = [
  {
    name: 'AppendCanonicalThreadMessage',
    url: '/api/workbench/threads/:thread_id/messages',
    method: 'POST',
    reqMapping: {
      path: ['thread_id'],
      header: ['X-Coze-Space-ID'],
      body: ['run_id', 'role', 'content', 'metadata', 'append_mode'],
    },
  },
  {
    name: 'GenerateCanonicalThreadSuggestions',
    url: '/api/workbench/threads/:thread_id/suggestions',
    method: 'POST',
    reqMapping: {
      path: ['thread_id'],
      header: ['X-Coze-Space-ID'],
      body: ['n', 'model_name', 'model_type'],
    },
  },
  {
    name: 'ListCanonicalThreadUploads',
    url: '/api/workbench/threads/:thread_id/uploads',
    method: 'GET',
    reqMapping: {
      path: ['thread_id'],
      header: ['X-Coze-Space-ID'],
    },
  },
  {
    name: 'UploadCanonicalThreadFiles',
    url: '/api/workbench/threads/:thread_id/uploads',
    method: 'POST',
    reqMapping: {
      path: ['thread_id'],
      header: ['X-Coze-Space-ID'],
    },
  },
  {
    name: 'DeleteCanonicalThreadUpload',
    url: '/api/workbench/threads/:thread_id/uploads/:file_id',
    method: 'DELETE',
    reqMapping: {
      path: ['thread_id', 'file_id'],
      header: ['X-Coze-Space-ID'],
    },
  },
  {
    name: 'ListCanonicalThreadArtifacts',
    url: '/api/workbench/threads/:thread_id/artifacts',
    method: 'GET',
    reqMapping: {
      path: ['thread_id'],
      header: ['X-Coze-Space-ID'],
      query: ['run_id', 'deleted_only', 'limit', 'offset'],
    },
  },
  {
    name: 'GetCanonicalThreadArtifactContent',
    url: '/api/workbench/threads/:thread_id/artifacts/:artifact_id/content',
    method: 'GET',
    reqMapping: {
      path: ['thread_id', 'artifact_id'],
      header: ['X-Coze-Space-ID'],
      query: ['mode'],
    },
  },
  {
    name: 'GetCanonicalThreadArtifactSignedURL',
    url: '/api/workbench/threads/:thread_id/artifacts/:artifact_id/signed_url',
    method: 'GET',
    reqMapping: {
      path: ['thread_id', 'artifact_id'],
      header: ['X-Coze-Space-ID'],
      query: ['mode', 'ttl_seconds'],
    },
  },
  {
    name: 'DeleteCanonicalThreadArtifact',
    url: '/api/workbench/threads/:thread_id/artifacts/:artifact_id',
    method: 'DELETE',
    reqMapping: {
      path: ['thread_id', 'artifact_id'],
      header: ['X-Coze-Space-ID'],
    },
  },
  {
    name: 'RestoreCanonicalThreadArtifact',
    url: '/api/workbench/threads/:thread_id/artifacts/:artifact_id/restore',
    method: 'POST',
    reqMapping: {
      path: ['thread_id', 'artifact_id'],
      header: ['X-Coze-Space-ID'],
    },
  },
  {
    name: 'ReviewCanonicalThreadArtifactScan',
    url: '/api/workbench/threads/:thread_id/artifacts/:artifact_id/scan_review',
    method: 'POST',
    reqMapping: {
      path: ['thread_id', 'artifact_id'],
      header: ['X-Coze-Space-ID'],
      body: ['decision', 'reason'],
    },
  },
  {
    name: 'ListCanonicalThreadArtifactScanJobs',
    url: '/api/workbench/threads/:thread_id/artifact_scan_jobs',
    method: 'GET',
    reqMapping: {
      path: ['thread_id'],
      header: ['X-Coze-Space-ID'],
      query: ['run_id', 'artifact_id', 'status', 'scanner', 'limit', 'offset'],
    },
  },
  {
    name: 'RetryCanonicalThreadArtifactScanJob',
    url: '/api/workbench/threads/:thread_id/artifact_scan_jobs/:job_id/retry',
    method: 'POST',
    reqMapping: {
      path: ['thread_id', 'job_id'],
      header: ['X-Coze-Space-ID'],
    },
  },
  {
    name: 'GetCanonicalThreadTokenUsage',
    url: '/api/workbench/threads/:thread_id/token_usage',
    method: 'GET',
    reqMapping: {
      path: ['thread_id'],
      header: ['X-Coze-Space-ID'],
      query: ['run_id', 'include_child_runs', 'source', 'limit', 'offset'],
    },
  },
  {
    name: 'ListCanonicalThreadMemories',
    url: '/api/workbench/threads/:thread_id/memories',
    method: 'GET',
    reqMapping: {
      path: ['thread_id'],
      header: ['X-Coze-Space-ID'],
      query: [
        'run_id',
        'scope',
        'scopes',
        'q',
        'include_expired',
        'include_deleted',
        'limit',
        'offset',
      ],
    },
  },
  {
    name: 'UpdateCanonicalThreadMemory',
    url: '/api/workbench/threads/:thread_id/memories/:memory_id',
    method: 'PUT',
    reqMapping: {
      path: ['thread_id', 'memory_id'],
      header: ['X-Coze-Space-ID'],
      body: [
        'run_id',
        'scope',
        'content',
        'metadata',
        'score',
        'confidence',
        'source_type',
        'source_id',
        'correction_of_memory_id',
        'corrected_at',
        'expires_at',
      ],
    },
  },
  {
    name: 'DeleteCanonicalThreadMemory',
    url: '/api/workbench/threads/:thread_id/memories/:memory_id',
    method: 'DELETE',
    reqMapping: {
      path: ['thread_id', 'memory_id'],
      header: ['X-Coze-Space-ID'],
    },
  },
  {
    name: 'RestoreCanonicalThreadMemory',
    url: '/api/workbench/threads/:thread_id/memories/:memory_id/restore',
    method: 'POST',
    reqMapping: {
      path: ['thread_id', 'memory_id'],
      header: ['X-Coze-Space-ID'],
    },
  },
  {
    name: 'ClearCanonicalThreadMemories',
    url: '/api/workbench/threads/:thread_id/memories/clear',
    method: 'POST',
    reqMapping: {
      path: ['thread_id'],
      header: ['X-Coze-Space-ID'],
      body: ['run_id', 'scopes'],
    },
  },
  {
    name: 'ExportCanonicalThreadMemories',
    url: '/api/workbench/threads/:thread_id/memories/export',
    method: 'GET',
    reqMapping: {
      path: ['thread_id'],
      header: ['X-Coze-Space-ID'],
      query: [
        'run_id',
        'scope',
        'scopes',
        'q',
        'include_expired',
        'include_deleted',
        'limit',
      ],
    },
  },
  {
    name: 'ImportCanonicalThreadMemories',
    url: '/api/workbench/threads/:thread_id/memories/import',
    method: 'POST',
    reqMapping: {
      path: ['thread_id'],
      header: ['X-Coze-Space-ID'],
      body: ['memories'],
    },
  },
  {
    name: 'ListCanonicalThreadMemoryAuditEvents',
    url: '/api/workbench/threads/:thread_id/memories/audit_events',
    method: 'GET',
    reqMapping: {
      path: ['thread_id'],
      header: ['X-Coze-Space-ID'],
      query: ['memory_id', 'limit', 'offset'],
    },
  },
  {
    name: 'ListCanonicalThreadGuardrailAuditEvents',
    url: '/api/workbench/threads/:thread_id/guardrail_audit_events',
    method: 'GET',
    reqMapping: {
      path: ['thread_id'],
      header: ['X-Coze-Space-ID'],
      query: ['run_id', 'limit', 'offset'],
    },
  },
  {
    name: 'ExportCanonicalThreadGuardrailAuditEvents',
    url: '/api/workbench/threads/:thread_id/guardrail_audit_events/export',
    method: 'GET',
    reqMapping: {
      path: ['thread_id'],
      header: ['X-Coze-Space-ID'],
      query: ['run_id', 'limit', 'offset'],
    },
  },
  {
    name: 'ListCanonicalThreadMCPRuntimeAuditEvents',
    url: '/api/workbench/threads/:thread_id/mcp_runtime_audit_events',
    method: 'GET',
    reqMapping: {
      path: ['thread_id'],
      header: ['X-Coze-Space-ID'],
      query: ['run_id', 'limit', 'offset'],
    },
  },
  {
    name: 'RetryCanonicalSubagentRun',
    url: '/api/workbench/threads/:thread_id/runs/:run_id/retry',
    method: 'POST',
    reqMapping: {
      path: ['thread_id', 'run_id'],
      header: ['X-Coze-Space-ID', 'Idempotency-Key'],
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
      ['thread_id: string,', '"X-Coze-Space-ID": string,'].join('\n  '),
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

  it('generates the typed canonical message projection', () => {
    expect(interfaceSource('CanonicalMessage').trim()).toBe(
      [
        'message_id: string,',
        'thread_id: string,',
        'run_id: string,',
        'role: string,',
        'content: string,',
        'metadata: any,',
        'created_at: string,',
        'seq?: string,',
      ].join('\n  '),
    );
    expect(interfaceSource('CanonicalMessagePage')).toMatch(
      /data:\s*CanonicalMessage\[\][,;]/,
    );
    expect(interfaceSource('CanonicalMessagePage')).not.toMatch(
      /data:\s*any[,;]/,
    );
  });

  it('exports exactly the 47 canonical createAPI functions', () => {
    const generatedAPIFunctions = Array.from(
      generatedSource.matchAll(
        /export const (\w+) = \/\*#__PURE__\*\/createAPI</g,
      ),
      match => match[1],
    );

    expect(generatedAPIFunctions).toEqual([
      ...canonicalAPIFunctions,
      ...productMethods,
    ]);
    for (const functionName of canonicalAPIFunctions) {
      expect(api[functionName]).toBeTypeOf('function');
    }
    for (const method of productMethods) {
      expect(api[method]).toBeTypeOf('function');
    }
  });

  it('binds stable upload IDs to the canonical upload delete method', () => {
    const config = apiConfig('DeleteCanonicalThreadUpload');

    expect({ url: config.url, method: config.method }).toEqual({
      url: '/api/workbench/threads/:thread_id/uploads/:file_id',
      method: 'DELETE',
    });
  });

  it('does not import legacy task generated contracts', () => {
    expect(generatedSource).not.toContain('workbench/task');
    expect(generatedSource).not.toMatch(
      /^import\s+(?:[^;\n]+\s+from\s+)?['"]\.\/task['"];?\s*$/m,
    );
  });

  it('maps the workspace header on every canonical core request', () => {
    for (const name of canonicalAPIFunctions) {
      expect(apiConfig(name).reqMapping?.header ?? [], name).toContain(
        'X-Coze-Space-ID',
      );
    }
  });

  it('freezes every canonical method, path, and request mapping', () => {
    const runMapping = {
      path: ['thread_id'],
      body: [...canonicalRunBody],
      header: ['Idempotency-Key', 'X-Coze-Space-ID'],
    };
    const waitMapping = {
      ...runMapping,
      body: [...canonicalRunBody, 'raise_error'],
    };

    const expectedConfigs = [
      ...canonicalAPIConfigs.map(expected => ({
        ...expected,
        reqMapping:
          expected.name === 'CreateCanonicalRun' ||
          expected.name === 'StreamCanonicalRun'
            ? runMapping
            : expected.name === 'WaitCanonicalRun'
              ? waitMapping
              : expected.reqMapping,
      })),
      ...productAPIConfigs,
    ];
    const actualConfigs = [...canonicalAPIFunctions, ...productMethods].map(
      name => {
        const actual = apiConfig(name);
        return {
          name: actual.name,
          url: actual.url,
          method: actual.method,
          reqMapping: actual.reqMapping,
        };
      },
    );

    expect(actualConfigs).toEqual(expectedConfigs);
    for (const config of actualConfigs) {
      expect(config.reqMapping?.header).toContain('X-Coze-Space-ID');
    }

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
