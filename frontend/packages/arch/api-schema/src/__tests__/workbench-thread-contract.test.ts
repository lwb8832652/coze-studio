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

import { readFileSync, readdirSync } from 'node:fs';

import { describe, expect, it } from 'vitest';
import ts from 'typescript';

import * as api from '../idl/workbench/thread';
import * as scheduledTaskAPI from '../idl/workbench/task';

const generatedSource = readFileSync(
  new URL('../idl/workbench/thread.ts', import.meta.url),
  'utf8',
);
const generatedProductSource = readFileSync(
  new URL('../idl/workbench/thread_product.ts', import.meta.url),
  'utf8',
);
const generatedTaskSource = readFileSync(
  new URL('../idl/workbench/task.ts', import.meta.url),
  'utf8',
);
const threadSourceFile = ts.createSourceFile(
  'thread.ts',
  generatedSource,
  ts.ScriptTarget.Latest,
  true,
  ts.ScriptKind.TS,
);
const threadProductSourceFile = ts.createSourceFile(
  'thread_product.ts',
  generatedProductSource,
  ts.ScriptTarget.Latest,
  true,
  ts.ScriptKind.TS,
);
const taskSourceFile = ts.createSourceFile(
  'task.ts',
  generatedTaskSource,
  ts.ScriptTarget.Latest,
  true,
  ts.ScriptKind.TS,
);
const generatedSourceFiles = [threadSourceFile, threadProductSourceFile];
const generatedWorkbenchDirectory = new URL(
  '../idl/workbench/',
  import.meta.url,
);
const allGeneratedWorkbenchSourceFiles = readdirSync(
  generatedWorkbenchDirectory,
  { withFileTypes: true },
)
  .filter(entry => entry.isFile() && entry.name.endsWith('.ts'))
  .sort((left, right) => left.name.localeCompare(right.name))
  .map(entry =>
    ts.createSourceFile(
      entry.name,
      readFileSync(new URL(entry.name, generatedWorkbenchDirectory), 'utf8'),
      ts.ScriptTarget.Latest,
      true,
      ts.ScriptKind.TS,
    ),
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

const scheduledTaskAPIFunctions = [
  'CreateScheduledTask',
  'ListScheduledTasks',
  'ListScheduledTaskTargets',
  'ListScheduledTaskCronPresets',
  'GetScheduledTask',
  'UpdateScheduledTask',
  'DeleteScheduledTask',
  'EnableScheduledTask',
  'DisableScheduledTask',
  'ExecuteScheduledTask',
  'ListScheduledTaskExecutions',
] as const;

const scheduledTaskContractTypes = [
  'ScheduledTaskTargetType',
  'ScheduledTaskScheduleType',
  'ScheduledTaskStatus',
  'ScheduledTaskExecutionStatus',
  'ScheduledTask',
  'ScheduledTaskExecution',
  'ScheduledTaskTarget',
  'ScheduledTaskCronPreset',
  'CreateScheduledTaskRequest',
  'UpdateScheduledTaskRequest',
  'GetScheduledTaskRequest',
  'ScheduledTaskActionRequest',
  'ListScheduledTasksRequest',
  'ListScheduledTaskExecutionsRequest',
  'ListScheduledTaskTargetsRequest',
  'ListScheduledTaskCronPresetsRequest',
  'ScheduledTaskResponse',
  'ScheduledTaskExecutionResponse',
  'ListScheduledTasksData',
  'ListScheduledTasksResponse',
  'ListScheduledTaskExecutionsData',
  'ListScheduledTaskExecutionsResponse',
  'ListScheduledTaskTargetsData',
  'ListScheduledTaskTargetsResponse',
  'ListScheduledTaskCronPresetsResponse',
] as const;

const scheduledTaskAPIConfigs = [
  {
    name: 'CreateScheduledTask',
    url: '/api/workbench/scheduled_tasks',
    method: 'POST',
  },
  {
    name: 'ListScheduledTasks',
    url: '/api/workbench/scheduled_tasks',
    method: 'GET',
  },
  {
    name: 'ListScheduledTaskTargets',
    url: '/api/workbench/scheduled_task_targets',
    method: 'GET',
  },
  {
    name: 'ListScheduledTaskCronPresets',
    url: '/api/workbench/scheduled_task_cron_presets',
    method: 'GET',
  },
  {
    name: 'GetScheduledTask',
    url: '/api/workbench/scheduled_tasks/:task_id',
    method: 'GET',
  },
  {
    name: 'UpdateScheduledTask',
    url: '/api/workbench/scheduled_tasks/:task_id',
    method: 'PUT',
  },
  {
    name: 'DeleteScheduledTask',
    url: '/api/workbench/scheduled_tasks/:task_id',
    method: 'DELETE',
  },
  {
    name: 'EnableScheduledTask',
    url: '/api/workbench/scheduled_tasks/:task_id/enable',
    method: 'POST',
  },
  {
    name: 'DisableScheduledTask',
    url: '/api/workbench/scheduled_tasks/:task_id/disable',
    method: 'POST',
  },
  {
    name: 'ExecuteScheduledTask',
    url: '/api/workbench/scheduled_tasks/:task_id/execute',
    method: 'POST',
  },
  {
    name: 'ListScheduledTaskExecutions',
    url: '/api/workbench/scheduled_tasks/:task_id/executions',
    method: 'GET',
  },
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

function apiTypeArguments(name: string): {
  request: string;
  response: string;
} {
  const declaration = threadSourceFile.statements
    .filter(ts.isVariableStatement)
    .filter(statement =>
      statement.modifiers?.some(
        modifier => modifier.kind === ts.SyntaxKind.ExportKeyword,
      ),
    )
    .flatMap(statement => [...statement.declarationList.declarations])
    .find(
      candidate =>
        ts.isIdentifier(candidate.name) && candidate.name.text === name,
    );
  expect(declaration, `${name} must be generated`).toBeDefined();

  const initializer = declaration?.initializer;
  expect(
    initializer && ts.isCallExpression(initializer),
    `${name} must initialize with a createAPI call`,
  ).toBe(true);
  if (!initializer || !ts.isCallExpression(initializer)) {
    return { request: '', response: '' };
  }

  expect(
    ts.isIdentifier(initializer.expression) &&
      initializer.expression.text === 'createAPI',
    `${name} must call createAPI`,
  ).toBe(true);

  const typeArguments = initializer.typeArguments ?? [];
  expect(
    typeArguments,
    `${name} must declare request and response types`,
  ).toHaveLength(2);

  return {
    request:
      typeArguments[0]?.getText(threadSourceFile).replace(/\s+/g, '') ?? '',
    response:
      typeArguments[1]?.getText(threadSourceFile).replace(/\s+/g, '') ?? '',
  };
}

function taskContractImportSpecifiers(sourceFile: ts.SourceFile): string[] {
  return sourceFile.statements
    .filter(ts.isImportDeclaration)
    .map(statement => statement.moduleSpecifier)
    .filter(ts.isStringLiteral)
    .map(moduleSpecifier => moduleSpecifier.text)
    .filter(
      moduleSpecifier =>
        moduleSpecifier === './task' ||
        moduleSpecifier.includes('workbench/task'),
    );
}

function taskThreadIdentifiers(sourceFile: ts.SourceFile): string[] {
  const identifiers = new Set<string>();
  const visit = (node: ts.Node): void => {
    if (ts.isIdentifier(node) && node.text.includes('TaskThread')) {
      identifiers.add(node.text);
    }
    ts.forEachChild(node, visit);
  };

  visit(sourceFile);
  return [...identifiers];
}

const retiredChatTaskIdentifierPrefixes = [
  'CancelTask',
  'ChatTask',
  'GetTask',
  'ListTaskEvents',
  'ListTasks',
  'RetryTask',
  'TaskEvent',
  'TaskStatus',
  'WorkbenchChat',
] as const;

function retiredChatTaskIdentifiers(sourceFile: ts.SourceFile): string[] {
  const identifiers = new Set<string>();
  const visit = (node: ts.Node): void => {
    if (
      ts.isIdentifier(node) &&
      retiredChatTaskIdentifierPrefixes.some(prefix =>
        node.text.startsWith(prefix),
      )
    ) {
      identifiers.add(node.text);
    }
    ts.forEachChild(node, visit);
  };

  visit(sourceFile);
  return [...identifiers];
}

const retiredWorkbenchRoutePatterns = [
  /\/api\/workbench\/task_threads\b/g,
  /\/api\/workbench\/tasks\b/g,
  /\/api\/workbench\/chat\b/g,
  /\/api\/threads\b/g,
  /\/api\/runs\b/g,
];

function retiredWorkbenchRouteFindings(sourceFile: ts.SourceFile): string[] {
  return retiredWorkbenchRoutePatterns.flatMap(pattern =>
    Array.from(sourceFile.text.matchAll(pattern), match => match[0]),
  );
}

function assertNoTaskThreadContracts(sourceFile: ts.SourceFile): void {
  expect(
    taskContractImportSpecifiers(sourceFile),
    `${sourceFile.fileName} must not import task contracts`,
  ).toEqual([]);
  expect(
    taskThreadIdentifiers(sourceFile),
    `${sourceFile.fileName} must not reference TaskThread identifiers`,
  ).toEqual([]);
}

function exportedInterfaceAndEnumNames(sourceFile: ts.SourceFile): string[] {
  return sourceFile.statements.flatMap(statement => {
    if (
      !ts.isInterfaceDeclaration(statement) &&
      !ts.isEnumDeclaration(statement)
    ) {
      return [];
    }
    if (
      !statement.modifiers?.some(
        modifier => modifier.kind === ts.SyntaxKind.ExportKeyword,
      )
    ) {
      return [];
    }
    return [statement.name.text];
  });
}

function apiConfigFromSource(
  source: string,
  name: string,
): {
  url: string;
  method: string;
  name: string;
  reqMapping?: Record<string, string[]>;
} {
  const declarationStart = source.indexOf(`export const ${name} =`);
  expect(declarationStart, `${name} must be generated`).toBeGreaterThanOrEqual(
    0,
  );

  const objectStart = source.indexOf('>({', declarationStart) + 2;
  const objectEnd = source.indexOf('\n});', objectStart);
  expect(objectStart, `${name} config must start`).toBeGreaterThanOrEqual(2);
  expect(objectEnd, `${name} config must end`).toBeGreaterThan(objectStart);

  return JSON.parse(source.slice(objectStart, objectEnd + 2));
}

function apiConfig(name: string): {
  url: string;
  method: string;
  name: string;
  reqMapping?: Record<string, string[]>;
} {
  return apiConfigFromSource(generatedSource, name);
}

describe('Scheduled Task generated contract', () => {
  it('keeps exactly the Scheduled Task DTO and enum exports', () => {
    expect(exportedInterfaceAndEnumNames(taskSourceFile)).toEqual(
      scheduledTaskContractTypes,
    );
    expect(taskThreadIdentifiers(taskSourceFile)).toEqual([]);
  });

  it('keeps exactly the Scheduled Task methods and routes', () => {
    const generatedAPIFunctions = Array.from(
      generatedTaskSource.matchAll(
        /export const (\w+) = \/\*#__PURE__\*\/createAPI/g,
      ),
      match => match[1],
    );
    expect(generatedAPIFunctions).toEqual(scheduledTaskAPIFunctions);

    for (const functionName of scheduledTaskAPIFunctions) {
      expect(scheduledTaskAPI[functionName]).toBeTypeOf('function');
    }
    expect(
      scheduledTaskAPIFunctions.map(name => {
        const config = apiConfigFromSource(generatedTaskSource, name);
        return { name: config.name, url: config.url, method: config.method };
      }),
    ).toEqual(scheduledTaskAPIConfigs);
    expect(generatedTaskSource).not.toContain('/api/workbench/task_threads');
  });

  it('keeps retired TaskThread and ChatTask contracts out of generated modules', () => {
    for (const sourceFile of allGeneratedWorkbenchSourceFiles) {
      expect(taskThreadIdentifiers(sourceFile), sourceFile.fileName).toEqual(
        [],
      );
      expect(
        retiredChatTaskIdentifiers(sourceFile),
        sourceFile.fileName,
      ).toEqual([]);
      expect(
        retiredWorkbenchRouteFindings(sourceFile),
        sourceFile.fileName,
      ).toEqual([]);
    }
    expect(scheduledTaskAPIFunctions).toHaveLength(11);
  });
});

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
    expect(apiTypeArguments('AppendCanonicalThreadMessage')).toEqual({
      request: 'thread_product.AppendCanonicalThreadMessageRequest',
      response: 'CanonicalMessage',
    });
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

  it('does not import or reference legacy task generated contracts', () => {
    for (const sourceFile of generatedSourceFiles) {
      assertNoTaskThreadContracts(sourceFile);
    }
    expect(interfaceSource('CanonicalThreadState')).toMatch(
      /\btasks:\s*any[,;]/,
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
      body: [...canonicalRunBody, 'coze'],
      header: ['Idempotency-Key', 'X-Coze-Space-ID'],
    };
    const waitMapping = {
      ...runMapping,
      body: [...canonicalRunBody, 'raise_error', 'coze'],
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
