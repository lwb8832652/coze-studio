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

import type * as journalContract from '../workbench-journal';
import * as api from '../idl/workbench/thread';
import * as scheduledTaskAPI from '../idl/workbench/task';
import * as journal from '../idl/workbench/journal';

const untypedVersionedJournalEvent = {
  event_id: 'event-1',
  thread_id: 'thread-1',
  run_id: 'run-1',
  event_type: 'action.started',
  payload: { unexpected: true },
  payload_version: journal.JOURNAL_PAYLOAD_VERSION,
  created_at: '2026-07-30T14:32:10.123456789Z',
};

// @ts-expect-error versioned Journal events require a typed payload branch
const rejectedUntypedJournalEvent: journalContract.JournalEvent =
  untypedVersionedJournalEvent;
void rejectedUntypedJournalEvent;

const mismatchedSnapshotEnvelope = {
  content_type: journal.JournalSnapshotContentType.Document,
  snapshot_id: 'snapshot-1',
  event_id: 'event-1',
  attempt_id: 'attempt-1',
  is_fragmented: false,
  status: journal.JournalContentStatus.Ready,
  created_at: '2026-07-30T14:32:10.123456789Z',
  visibility: journal.JournalVisibility.User,
  fragments: [],
  has_more: false,
  content: { terminal: { command: 'pwd' } },
};

// @ts-expect-error content_type must match the exclusive snapshot content branch
const rejectedMismatchedSnapshot: journalContract.JournalSnapshotEnvelope =
  mismatchedSnapshotEnvelope;
void rejectedMismatchedSnapshot;

const loadingSnapshotWithoutContent: journalContract.JournalSnapshotEnvelope = {
  content_type: journal.JournalSnapshotContentType.Document,
  snapshot_id: 'snapshot-loading',
  event_id: 'event-loading',
  attempt_id: 'attempt-1',
  is_fragmented: false,
  status: journal.JournalContentStatus.Loading,
  created_at: '2026-07-30T14:32:10.123456789Z',
  visibility: journal.JournalVisibility.User,
  fragments: [],
  has_more: false,
};
void loadingSnapshotWithoutContent;

const readySnapshotWithoutContent = {
  ...loadingSnapshotWithoutContent,
  snapshot_id: 'snapshot-ready',
  event_id: 'event-ready',
  status: journal.JournalContentStatus.Ready,
};

// @ts-expect-error ready snapshots require their matching typed content branch
const rejectedReadySnapshotWithoutContent: journalContract.JournalSnapshotEnvelope =
  readySnapshotWithoutContent;
void rejectedReadySnapshotWithoutContent;

const generatedSource = readFileSync(
  new URL('../idl/workbench/thread.ts', import.meta.url),
  'utf8',
);
const generatedProductSource = readFileSync(
  new URL('../idl/workbench/thread_product.ts', import.meta.url),
  'utf8',
);
const generatedJournalSource = readFileSync(
  new URL('../idl/workbench/journal.ts', import.meta.url),
  'utf8',
);
const journalContractSource = readFileSync(
  new URL('../workbench-journal.ts', import.meta.url),
  'utf8',
);
const packageIndexSource = readFileSync(
  new URL('../index.ts', import.meta.url),
  'utf8',
);
const adminConfigThriftSource = readFileSync(
  new URL('../../../../../../idl/admin/config.thrift', import.meta.url),
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
const journalSourceFile = ts.createSourceFile(
  'journal.ts',
  generatedJournalSource,
  ts.ScriptTarget.Latest,
  true,
  ts.ScriptKind.TS,
);
const journalContractSourceFile = ts.createSourceFile(
  'workbench-journal.ts',
  journalContractSource,
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

const journalMethods = [
  'GetCanonicalRunJournal',
  'GetCanonicalRunSnapshot',
  'AuditCanonicalRunSnapshotAction',
  'RecoverCanonicalRunJournal',
  'GetCanonicalJournalSettings',
  'PatchCanonicalJournalSettings',
  'CopyCanonicalThreadArtifactLink',
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
      query: [
        'after_event_id',
        'cancel_on_disconnect',
        'stream_mode',
        'journal_protocol_version',
      ],
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
      query: [
        'after_event_id',
        'event_types',
        'limit',
        'attempt_id',
        'after_sequence',
      ],
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
      query: ['run_id', 'deleted_only', 'limit', 'offset', 'collection_id'],
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

const journalAPIConfigs: CanonicalAPIExpectation[] = [
  {
    name: 'GetCanonicalRunJournal',
    url: '/api/workbench/threads/:thread_id/runs/:run_id/journal',
    method: 'GET',
    reqMapping: {
      path: ['thread_id', 'run_id'],
      header: ['X-Coze-Space-ID'],
      query: [
        'attempt_id',
        'after_sequence',
        'limit',
        'journal_protocol_version',
        'after_event_id',
      ],
    },
  },
  {
    name: 'GetCanonicalRunSnapshot',
    url: '/api/workbench/threads/:thread_id/runs/:run_id/snapshots/:snapshot_id',
    method: 'GET',
    reqMapping: {
      path: ['thread_id', 'run_id', 'snapshot_id'],
      header: ['X-Coze-Space-ID'],
      query: ['cursor', 'limit'],
    },
  },
  {
    name: 'AuditCanonicalRunSnapshotAction',
    url: '/api/workbench/threads/:thread_id/runs/:run_id/snapshots/:snapshot_id/actions',
    method: 'POST',
    reqMapping: {
      path: ['thread_id', 'run_id', 'snapshot_id'],
      header: ['X-Coze-Space-ID', 'Idempotency-Key'],
      body: ['action', 'fragment_id'],
    },
  },
  {
    name: 'RecoverCanonicalRunJournal',
    url: '/api/workbench/threads/:thread_id/runs/:run_id/recover',
    method: 'POST',
    reqMapping: {
      path: ['thread_id', 'run_id'],
      header: ['X-Coze-Space-ID', 'Idempotency-Key'],
      body: ['source_attempt_id', 'action', 'confirmed'],
    },
  },
  {
    name: 'GetCanonicalJournalSettings',
    url: '/api/workbench/journal/settings',
    method: 'GET',
    reqMapping: {},
  },
  {
    name: 'PatchCanonicalJournalSettings',
    url: '/api/workbench/journal/settings',
    method: 'PATCH',
    reqMapping: {
      body: ['split_ratio', 'revision'],
    },
  },
  {
    name: 'CopyCanonicalThreadArtifactLink',
    url: '/api/workbench/threads/:thread_id/artifacts/:artifact_id/copy_link',
    method: 'POST',
    reqMapping: {
      path: ['thread_id', 'artifact_id'],
      header: ['X-Coze-Space-ID'],
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
  return interfaceSourceFrom(generatedSource, name);
}

function interfaceSourceFrom(source: string, name: string): string {
  const match = source.match(
    new RegExp(`export interface ${name} \\{([\\s\\S]*?)\\n\\}`),
  );

  expect(match, `${name} must be generated`).not.toBeNull();
  return match?.[1] ?? '';
}

function declarationSourceFrom(
  sourceFile: ts.SourceFile,
  source: string,
  name: string,
): string {
  const declaration = sourceFile.statements.find(
    statement =>
      (ts.isInterfaceDeclaration(statement) ||
        ts.isTypeAliasDeclaration(statement)) &&
      statement.name.text === name,
  );

  expect(declaration, `${name} must be generated`).not.toBeUndefined();
  return declaration
    ? source.slice(declaration.getStart(sourceFile), declaration.end)
    : '';
}

function thriftStructSourceFrom(source: string, name: string): string {
  const match = source.match(
    new RegExp(`struct\\s+${name}\\s*\\{([\\s\\S]*?)\\n\\}`),
  );

  expect(match, `${name} must be declared`).not.toBeNull();
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

  it('exports exactly the 54 canonical createAPI functions', () => {
    const generatedAPIFunctions = Array.from(
      generatedSource.matchAll(
        /export const (\w+) = \/\*#__PURE__\*\/createAPI</g,
      ),
      match => match[1],
    );

    expect(generatedAPIFunctions).toEqual([
      ...canonicalAPIFunctions,
      ...productMethods,
      ...journalMethods,
    ]);
    for (const functionName of canonicalAPIFunctions) {
      expect(api[functionName]).toBeTypeOf('function');
    }
    for (const method of productMethods) {
      expect(api[method]).toBeTypeOf('function');
    }
    for (const method of journalMethods) {
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
      ...journalAPIConfigs,
    ];
    const actualConfigs = [
      ...canonicalAPIFunctions,
      ...productMethods,
      ...journalMethods,
    ].map(name => {
      const actual = apiConfig(name);
      return {
        name: actual.name,
        url: actual.url,
        method: actual.method,
        reqMapping: actual.reqMapping,
      };
    });

    expect(actualConfigs).toEqual(expectedConfigs);
    for (const config of actualConfigs.filter(
      item =>
        item.name !== 'GetCanonicalJournalSettings' &&
        item.name !== 'PatchCanonicalJournalSettings',
    )) {
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

  it('freezes typed Journal events, payloads, snapshots, and settings', () => {
    expect(journal.JOURNAL_SCHEMA_VERSION).toBe('1.1');
    expect(journal.JOURNAL_PAYLOAD_VERSION).toBe('1.0');
    expect(journal.JOURNAL_PROTOCOL_VERSION).toBe('1.1');
    expect(journal.JOURNAL_SPLIT_RATIO_MIN).toBe(0.4);
    expect(journal.JOURNAL_SPLIT_RATIO_MAX).toBe(0.7);

    const event = interfaceSourceFrom(generatedJournalSource, 'JournalEvent');
    expect(event).toMatch(/event_id:\s*string[,;]/);
    expect(event).toMatch(/thread_id:\s*string[,;]/);
    expect(event).toMatch(/run_id:\s*string[,;]/);
    expect(event).toMatch(/event_type:\s*string[,;]/);
    expect(event).toMatch(/payload:\s*any[,;]/);
    expect(event).toMatch(/created_at:\s*string[,;]/);
    expect(event).toMatch(/schema_version\?:\s*string[,;]/);
    expect(event).toMatch(/payload_version\?:\s*string[,;]/);
    expect(event).toMatch(/attempt_id\?:\s*string[,;]/);
    expect(event).toMatch(/sequence\?:\s*number[,;]/);

    for (const payloadName of [
      'JournalMilestoneEventPayload',
      'JournalActionEventPayload',
      'JournalArtifactEventPayload',
      'JournalVerificationEventPayload',
      'JournalConfirmationEventPayload',
    ]) {
      const payload = declarationSourceFrom(
        journalSourceFile,
        generatedJournalSource,
        payloadName,
      );
      expect(payload, payloadName).toMatch(
        /type:\s*Journal(?:Milestone|Action|Artifact|Verification|Confirmation)PayloadType[,;]/,
      );
      expect(payload, payloadName).toMatch(/data:\s*Journal\w+EventData[,;]/);
      expect(payload, payloadName).not.toMatch(/data\??:\s*any[,;]/);
    }
    expect(journalContractSource).toContain(
      'export type JournalTypedEventPayload =',
    );
    expect(journalContractSource).toContain('export type JournalEvent =');
    expect(journalContractSource).toContain("'payload' | 'payload_version'");
    expect(generatedJournalSource).not.toContain(
      'export enum JournalPayloadType',
    );
    expect(journal.JournalActionPayloadType.Terminal).toBe('terminal');
    expect(journal.JournalActionPayloadType.Document).toBe('document');
    expect(journal.JournalActionPayloadType.Generic).toBe('generic');

    const actionPayload = declarationSourceFrom(
      journalContractSourceFile,
      journalContractSource,
      'JournalActionPayload',
    );
    for (const kind of ['Document', 'Terminal', 'Code', 'Skill', 'Browser']) {
      expect(actionPayload, `${kind} action branch`).toMatch(
        new RegExp(
          `JournalActionPayloadType\\.${kind},[\\s\\S]*?JournalSnapshotContentType\\.${kind}`,
        ),
      );
    }
    expect(actionPayload).toContain('JournalActionPayloadType.Generic');
    expect(actionPayload).toContain('content_type?: never');

    const snapshot = declarationSourceFrom(
      journalSourceFile,
      generatedJournalSource,
      'JournalSnapshotEnvelope',
    );
    for (const field of [
      'content_type',
      'snapshot_id',
      'event_id',
      'attempt_id',
      'is_fragmented',
      'status',
      'created_at',
      'visibility',
      'error_code',
      'fragments',
      'has_more',
      'next_cursor',
      'content',
    ]) {
      expect(snapshot, field).toContain(field);
    }
    expect(snapshot).not.toMatch(/\bdata\??:\s*any[,;]/);
    expect(snapshot).toMatch(/content\?:\s*JournalSnapshotContent[,;]/);
    expect(snapshot).not.toMatch(
      /\n\s*(document|terminal|code|skill|browser)\??:/,
    );

    const snapshotStructs = {
      JournalSnapshotFragment: [
        'fragment_id',
        'fragment_index',
        'content',
        'byte_start',
        'byte_end',
        'size_bytes',
        'content_hash',
        'kind',
        'block_id',
        'stream',
        'start_line',
        'end_line',
        'item_start',
        'item_end',
        'binary_content_base64',
        'mime_type',
        'chapters',
        'highlights',
        'skills',
        'analysis',
      ],
      JournalDocumentChapter: ['chapter_id', 'title', 'level'],
      JournalDocumentSnapshotContent: [
        'title',
        'format',
        'content',
        'source_artifact_id',
        'token',
        'chapters',
        'active_block',
        'revision',
        'sync_status',
      ],
      JournalTerminalSnapshotContent: [
        'command',
        'output',
        'exit_code',
        'working_directory',
        'session_id',
        'started_at',
        'finished_at',
        'stdout',
        'stderr',
        'duration_ms',
      ],
      JournalCodeHighlight: ['start_line', 'end_line', 'kind'],
      JournalCodeSnapshotContent: [
        'file_path',
        'language',
        'content',
        'diff',
        'start_line',
        'end_line',
        'repository',
        'revision',
        'highlights',
      ],
      JournalBrowserSnapshotContent: [
        'url',
        'title',
        'screenshot_artifact_id',
        'capture_id',
        'thumbnail_base64',
        'static_snapshot_base64',
        'mime_type',
        'analysis',
        'index',
        'total',
        'redacted',
        'redaction_evidence_id',
        'redaction_policy_version',
      ],
      AuditCanonicalRunSnapshotActionRequest: ['fragment_id'],
      JournalSnapshotActionAuditResponse: [
        'copy_text',
        'download_url',
        'download_content_base64',
        'download_mime_type',
      ],
    };
    for (const [name, fields] of Object.entries(snapshotStructs)) {
      const declaration = declarationSourceFrom(
        journalSourceFile,
        generatedJournalSource,
        name,
      );
      for (const field of fields) {
        expect(declaration, `${name}.${field}`).toContain(field);
      }
    }
    expect(generatedJournalSource).not.toContain('original_object_key');
    expect(generatedJournalSource).not.toContain('original_url');
    expect(generatedJournalSource).not.toContain('Blob');
    const browserSnapshot = declarationSourceFrom(
      journalSourceFile,
      generatedJournalSource,
      'JournalBrowserSnapshotContent',
    );
    expect(browserSnapshot).not.toMatch(/\bcontent\??:/);
    expect(Object.values(journal.JournalSnapshotFragmentKind)).toEqual([
      'document_block',
      'document_chapters',
      'terminal_stdout',
      'terminal_stderr',
      'code_lines',
      'code_highlights',
      'skill_items',
      'browser_thumbnail',
      'browser_snapshot',
      'browser_analysis',
    ]);
    expect(packageIndexSource).toContain(
      "export * as workbenchJournal from './workbench-journal';",
    );

    const snapshotContent = declarationSourceFrom(
      journalContractSourceFile,
      journalContractSource,
      'JournalSnapshotContent',
    );
    expect(snapshotContent).toContain('export type JournalSnapshotContent =');
    expect(journalContractSource).toContain(
      'Exclude<keyof JournalSnapshotContentMap, TKey>]?: never',
    );
    expect(journalContractSource).toContain("'content_type' | 'content'");
    for (const branchName of [
      'Document',
      'Terminal',
      'Code',
      'Skill',
      'Browser',
    ]) {
      expect(journalContractSource).toContain(
        `[JournalSnapshotContentType.${branchName}]:`,
      );
    }

    for (const controlType of [
      'journal_disabled',
      'journal_degraded',
      'capability_unavailable',
      'protocol_incompatible',
    ]) {
      expect(Object.values(journal.JournalControlFrameType)).toContain(
        controlType,
      );
    }
    expect(journalContractSource).toContain('export type JournalStreamFrame =');

    expect(Object.values(journal.JournalErrorCode)).toEqual([
      'JOURNAL_CURSOR_EXPIRED',
      'JOURNAL_EVENT_GAP',
      'SNAPSHOT_UNAVAILABLE',
      'RESOURCE_NOT_FOUND',
      'RECOVERY_CONFLICT',
      'RECOVERY_CONFIRM_REQUIRED',
      'JOURNAL_RATE_LIMITED',
      'SCHEMA_INCOMPATIBLE',
      'NO_PERMISSION',
    ]);

    const skill = interfaceSourceFrom(generatedJournalSource, 'JournalSkill');
    expect(skill).toMatch(/skill_id:\s*string[,;]/);
    expect(skill).toMatch(/name:\s*string[,;]/);
    for (const field of [
      'invocation_status',
      'input_summary',
      'output_artifacts',
      'purpose_summary',
      'description',
    ]) {
      expect(skill, field).toContain(`${field}?`);
    }

    const patchSettings = interfaceSourceFrom(
      generatedJournalSource,
      'PatchCanonicalJournalSettingsRequest',
    );
    expect(patchSettings).toMatch(/split_ratio:\s*number[,;]/);
    expect(patchSettings).toMatch(/revision:\s*string[,;]/);
    expect(patchSettings).not.toMatch(/user_id/i);
    const bootstrapRequest = interfaceSourceFrom(
      generatedJournalSource,
      'GetCanonicalRunJournalRequest',
    );
    expect(bootstrapRequest).toMatch(/after_event_id\?:\s*string[,;]/);
    expect(generatedJournalSource).not.toMatch(/Support|Oncall|Ticket/);
  });

  it('freezes artifact enums, collection ordering, and runtime gates', () => {
    for (const enumName of [
      'CanonicalArtifactSource',
      'CanonicalArtifactGenerationStatus',
      'CanonicalArtifactPreviewMode',
      'CanonicalArtifactCapability',
    ]) {
      expect(generatedProductSource).toContain(`export enum ${enumName}`);
    }
    const artifact = interfaceSourceFrom(
      generatedProductSource,
      'CanonicalArtifact',
    );
    expect(artifact).toMatch(/source\?:\s*CanonicalArtifactSource[,;]/);
    expect(artifact).toMatch(
      /generation_status\?:\s*CanonicalArtifactGenerationStatus[,;]/,
    );
    expect(artifact).toMatch(
      /capabilities\?:\s*CanonicalArtifactCapability\[\][,;]/,
    );
    expect(artifact).toMatch(/collection_id\?:\s*string[,;]/);
    expect(artifact).toMatch(/collection_order\?:\s*number[,;]/);
    const collection = interfaceSourceFrom(
      generatedProductSource,
      'CanonicalArtifactCollection',
    );
    expect(collection).toMatch(/collection_id:\s*string[,;]/);
    expect(collection).toMatch(/artifact_ids:\s*string\[\][,;]/);

    const artifactList = interfaceSourceFrom(
      generatedProductSource,
      'CanonicalArtifactListResponse',
    );
    expect(artifactList).toMatch(
      /collections\?:\s*CanonicalArtifactCollection\[\][,;]/,
    );

    const runtimeConfig = thriftStructSourceFrom(
      adminConfigThriftSource,
      'JournalRuntimeConfiguration',
    );
    for (const field of [
      'journal_projection',
      'journal_ui',
      'journal_snapshots',
      'checkpoint_recovery',
      'journal_projection_rollout_basis_points',
      'journal_ui_rollout_basis_points',
      'journal_snapshots_rollout_basis_points',
      'checkpoint_recovery_rollout_basis_points',
      'sse_tenant_connection_cap',
      'sse_cluster_connection_cap',
      'sse_send_queue_high_watermark',
      'sse_send_queue_max',
      'short_request_qps',
      'short_request_burst',
      'lease_ttl_seconds',
      'snapshot_fragment_threshold_bytes',
      'config_revision',
    ]) {
      expect(runtimeConfig, field).toContain(field);
    }
  });
});
