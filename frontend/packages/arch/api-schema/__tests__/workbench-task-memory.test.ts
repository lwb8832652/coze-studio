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

import { describe, expect, it } from 'vitest';

import * as workbenchTask from '../src/idl/workbench/task';

describe('workbench task memory api schema', () => {
  it('exposes metadata-safe memory management API clients', () => {
    expect(typeof workbenchTask.ListTaskThreadMemories).toBe('function');
    expect(typeof workbenchTask.UpdateTaskThreadMemory).toBe('function');
    expect(typeof workbenchTask.DeleteTaskThreadMemory).toBe('function');
    expect(typeof workbenchTask.ClearTaskThreadMemories).toBe('function');
    expect(typeof workbenchTask.RestoreTaskThreadMemory).toBe('function');
    expect(typeof workbenchTask.ExportTaskThreadMemories).toBe('function');
    expect(typeof workbenchTask.ImportTaskThreadMemories).toBe('function');
    expect(typeof workbenchTask.ListTaskThreadMemoryAuditEvents).toBe(
      'function',
    );
    expect(typeof workbenchTask.ExportTaskThreadGuardrailAuditEvents).toBe(
      'function',
    );
  });

  it('maps memory management routes without exposing private execution fields', () => {
    expect(workbenchTask.ListTaskThreadMemories.meta).toMatchObject({
      method: 'GET',
      reqMapping: {
        path: ['thread_id'],
        query: [
          'run_id',
          'scope',
          'scopes',
          'q',
          'include_expired',
          'include_deleted',
          'page',
          'page_size',
        ],
      },
      url: '/api/workbench/task_threads/:thread_id/memories',
    });
    expect(workbenchTask.UpdateTaskThreadMemory.meta).toMatchObject({
      method: 'PUT',
      reqMapping: {
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
        path: ['thread_id', 'memory_id'],
      },
      url: '/api/workbench/task_threads/:thread_id/memories/:memory_id',
    });
    expect(workbenchTask.ListTaskThreadMemoryAuditEvents.meta).toMatchObject({
      method: 'GET',
      reqMapping: {
        path: ['thread_id'],
        query: ['memory_id', 'page', 'page_size'],
      },
      url: '/api/workbench/task_threads/:thread_id/memories/audit_events',
    });
    expect(workbenchTask.RestoreTaskThreadMemory.meta).toMatchObject({
      method: 'POST',
      reqMapping: {
        path: ['thread_id', 'memory_id'],
      },
      url: '/api/workbench/task_threads/:thread_id/memories/:memory_id/restore',
    });
    expect(workbenchTask.ExportTaskThreadMemories.meta).toMatchObject({
      method: 'GET',
      reqMapping: {
        path: ['thread_id'],
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
      url: '/api/workbench/task_threads/:thread_id/memories/export',
    });
    expect(
      workbenchTask.ExportTaskThreadGuardrailAuditEvents.meta,
    ).toMatchObject({
      method: 'GET',
      reqMapping: {
        path: ['thread_id'],
        query: ['run_id', 'page', 'page_size'],
      },
      url: '/api/workbench/task_threads/:thread_id/guardrail_audit_events/export',
    });
    expect(workbenchTask.ImportTaskThreadMemories.meta).toMatchObject({
      method: 'POST',
      reqMapping: {
        body: ['memories'],
        path: ['thread_id'],
      },
      url: '/api/workbench/task_threads/:thread_id/memories/import',
    });
  });
});
