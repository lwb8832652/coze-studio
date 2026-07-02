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

import { describe, expect, it, vi } from 'vitest';
import {
  workbench,
  workbenchSkill,
  workbenchTask,
} from '@coze-studio/api-schema';

import {
  appendTaskThreadMessage,
  createTaskThreadRun,
  deleteTaskThreadArtifact,
  fetchTaskThreadArtifactContent,
  getTaskThreadArtifactSignedURL,
  getTaskThread,
  getTaskThreadTokenUsage,
  listTaskThreadMemories,
  listTaskThreadArtifactScanJobs,
  listTaskThreadArtifacts,
  listTaskThreadRunEvents,
  listTaskThreadMessages,
  listTaskThreadRuns,
  listTaskThreads,
  reviewTaskThreadArtifactScan,
  retryTaskThreadArtifactScanJob,
  restoreTaskThreadArtifact,
  restoreTaskThreadMemory,
  resumeTaskThreadRun,
  updateTaskThreadMemory,
  deleteTaskThreadMemory,
  clearTaskThreadMemories,
  listTaskThreadMemoryAuditEvents,
  listTaskThreadGuardrailAuditEvents,
  exportTaskThreadMemories,
  exportTaskThreadGuardrailAuditEvents,
  getWorkbenchRuntimeDoctor,
  importTaskThreadMemories,
  installSkillFromArtifact,
} from '../service';

describe('task thread service', () => {
  it('exports task-thread API clients for the new task source', () => {
    expect(typeof listTaskThreads).toBe('function');
    expect(typeof getTaskThread).toBe('function');
    expect(typeof listTaskThreadMessages).toBe('function');
    expect(typeof appendTaskThreadMessage).toBe('function');
    expect(typeof listTaskThreadRuns).toBe('function');
    expect(typeof createTaskThreadRun).toBe('function');
    expect(typeof resumeTaskThreadRun).toBe('function');
    expect(typeof listTaskThreadRunEvents).toBe('function');
    expect(typeof getTaskThreadTokenUsage).toBe('function');
    expect(typeof listTaskThreadMemories).toBe('function');
    expect(typeof updateTaskThreadMemory).toBe('function');
    expect(typeof deleteTaskThreadMemory).toBe('function');
    expect(typeof clearTaskThreadMemories).toBe('function');
    expect(typeof restoreTaskThreadMemory).toBe('function');
    expect(typeof listTaskThreadMemoryAuditEvents).toBe('function');
    expect(typeof listTaskThreadGuardrailAuditEvents).toBe('function');
    expect(typeof exportTaskThreadMemories).toBe('function');
    expect(typeof exportTaskThreadGuardrailAuditEvents).toBe('function');
    expect(typeof importTaskThreadMemories).toBe('function');
    expect(typeof getWorkbenchRuntimeDoctor).toBe('function');
    expect(typeof listTaskThreadArtifacts).toBe('function');
    expect(typeof listTaskThreadArtifactScanJobs).toBe('function');
    expect(typeof retryTaskThreadArtifactScanJob).toBe('function');
    expect(typeof reviewTaskThreadArtifactScan).toBe('function');
    expect(typeof fetchTaskThreadArtifactContent).toBe('function');
    expect(typeof getTaskThreadArtifactSignedURL).toBe('function');
    expect(typeof deleteTaskThreadArtifact).toBe('function');
    expect(typeof restoreTaskThreadArtifact).toBe('function');
    expect(typeof installSkillFromArtifact).toBe('function');
  });

  it('exports Runtime Doctor client from generated workbench schema', () => {
    expect(getWorkbenchRuntimeDoctor).toBe(workbench.GetWorkbenchRuntimeDoctor);
    expect(getWorkbenchRuntimeDoctor.meta).toMatchObject({
      method: 'GET',
      reqMapping: {
        query: ['space_id'],
      },
      url: '/api/workbench/runtime_doctor',
    });
  });

  it('exports .skill artifact install client from generated workbenchSkill schema', () => {
    expect(installSkillFromArtifact).toBe(
      workbenchSkill.InstallSkillFromArtifact,
    );
    expect(installSkillFromArtifact.meta).toMatchObject({
      method: 'POST',
      reqMapping: {
        body: ['space_id', 'thread_id', 'artifact_id'],
      },
      url: '/api/workbench/skills/install',
    });
  });

  it('exports task memory clients from generated workbenchTask schema', () => {
    expect(listTaskThreadMemories).toBe(workbenchTask.ListTaskThreadMemories);
    expect(updateTaskThreadMemory).toBe(workbenchTask.UpdateTaskThreadMemory);
    expect(deleteTaskThreadMemory).toBe(workbenchTask.DeleteTaskThreadMemory);
    expect(clearTaskThreadMemories).toBe(workbenchTask.ClearTaskThreadMemories);
    expect(restoreTaskThreadMemory).toBe(workbenchTask.RestoreTaskThreadMemory);
    expect(listTaskThreadMemoryAuditEvents).toBe(
      workbenchTask.ListTaskThreadMemoryAuditEvents,
    );
    expect(listTaskThreadGuardrailAuditEvents).toBe(
      workbenchTask.ListTaskThreadGuardrailAuditEvents,
    );
    expect(exportTaskThreadMemories).toBe(
      workbenchTask.ExportTaskThreadMemories,
    );
    expect(exportTaskThreadGuardrailAuditEvents).toBe(
      workbenchTask.ExportTaskThreadGuardrailAuditEvents,
    );
    expect(importTaskThreadMemories).toBe(
      workbenchTask.ImportTaskThreadMemories,
    );
  });

  it('maps generated task memory API metadata', () => {
    expect(listTaskThreadMemories.meta).toMatchObject({
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
    expect(updateTaskThreadMemory.meta).toMatchObject({
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
    expect(deleteTaskThreadMemory.meta).toMatchObject({
      method: 'DELETE',
      reqMapping: {
        path: ['thread_id', 'memory_id'],
      },
      url: '/api/workbench/task_threads/:thread_id/memories/:memory_id',
    });
    expect(clearTaskThreadMemories.meta).toMatchObject({
      method: 'POST',
      reqMapping: {
        body: ['run_id', 'scopes'],
        path: ['thread_id'],
      },
      url: '/api/workbench/task_threads/:thread_id/memories/clear',
    });
    expect(restoreTaskThreadMemory.meta).toMatchObject({
      method: 'POST',
      reqMapping: {
        path: ['thread_id', 'memory_id'],
      },
      url: '/api/workbench/task_threads/:thread_id/memories/:memory_id/restore',
    });
    expect(listTaskThreadMemoryAuditEvents.meta).toMatchObject({
      method: 'GET',
      reqMapping: {
        path: ['thread_id'],
        query: ['memory_id', 'page', 'page_size'],
      },
      url: '/api/workbench/task_threads/:thread_id/memories/audit_events',
    });
    expect(listTaskThreadGuardrailAuditEvents.meta).toMatchObject({
      method: 'GET',
      reqMapping: {
        path: ['thread_id'],
        query: ['run_id', 'page', 'page_size'],
      },
      url: '/api/workbench/task_threads/:thread_id/guardrail_audit_events',
    });
    expect(exportTaskThreadMemories.meta).toMatchObject({
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
    expect(exportTaskThreadGuardrailAuditEvents.meta).toMatchObject({
      method: 'GET',
      reqMapping: {
        path: ['thread_id'],
        query: ['run_id', 'page', 'page_size'],
      },
      url: '/api/workbench/task_threads/:thread_id/guardrail_audit_events/export',
    });
    expect(importTaskThreadMemories.meta).toMatchObject({
      method: 'POST',
      reqMapping: {
        body: ['memories'],
        path: ['thread_id'],
      },
      url: '/api/workbench/task_threads/:thread_id/memories/import',
    });
  });

  it('lists artifact scan jobs with encoded filters and pagination', async () => {
    const payload = {
      data: {
        jobs: [
          {
            artifact_id: 'artifact/1',
            attempt_count: 3,
            available_at: 1717000400000,
            created_at: 1717000200000,
            ended_at: 1717000350000,
            file_id: 'file-1',
            job_id: 'scan-job-1',
            last_error: 'scanner unavailable',
            lease_expires_at: 0,
            run_id: 'run-1',
            scanner: 'clamav',
            space_id: 'space-1',
            started_at: 1717000300000,
            status: 'failed',
            thread_id: 'thread 1',
            updated_at: 1717000350000,
            user_id: 'user-1',
            worker_id: 'worker-a',
          },
        ],
        total: 1,
      },
      code: 0,
      msg: '',
    };
    const fetchMock = vi.fn().mockResolvedValue({
      headers: new Headers({
        'content-type': 'application/json',
      }),
      json: () => Promise.resolve(payload),
      ok: true,
    });
    const previousFetch = globalThis.fetch;
    globalThis.fetch = fetchMock;

    try {
      const response = await listTaskThreadArtifactScanJobs({
        artifact_id: 'artifact/1',
        page: 2,
        page_size: 20,
        scanner: 'clamav',
        status: 'failed',
        thread_id: 'thread 1',
      });

      expect(fetchMock).toHaveBeenCalledWith(
        '/api/workbench/task_threads/thread%201/artifact_scan_jobs?artifact_id=artifact%2F1&status=failed&scanner=clamav&page=2&page_size=20',
        expect.objectContaining({
          headers: expect.objectContaining({
            'x-requested-with': 'XMLHttpRequest',
          }),
          method: 'GET',
        }),
      );
      expect(response).toBe(payload);
    } finally {
      globalThis.fetch = previousFetch;
    }
  });

  it('retries an artifact scan job with encoded route params', async () => {
    const payload = {
      data: {
        job: {
          artifact_id: 'artifact/1',
          attempt_count: 3,
          available_at: 1717000400000,
          created_at: 1717000200000,
          ended_at: 0,
          file_id: 'file-1',
          job_id: 'scan-job/1',
          last_error: 'manual retry requested',
          lease_expires_at: 0,
          run_id: 'run-1',
          scanner: 'clamav',
          space_id: 'space-1',
          started_at: 0,
          status: 'pending',
          thread_id: 'thread 1',
          updated_at: 1717000400000,
          user_id: 'user-1',
          worker_id: '',
        },
        retried: true,
      },
      code: 0,
      msg: '',
    };
    const fetchMock = vi.fn().mockResolvedValue({
      headers: new Headers({
        'content-type': 'application/json',
      }),
      json: () => Promise.resolve(payload),
      ok: true,
    });
    const previousFetch = globalThis.fetch;
    globalThis.fetch = fetchMock;

    try {
      const response = await retryTaskThreadArtifactScanJob({
        job_id: 'scan-job/1',
        thread_id: 'thread 1',
      });

      expect(fetchMock).toHaveBeenCalledWith(
        '/api/workbench/task_threads/thread%201/artifact_scan_jobs/scan-job%2F1/retry',
        expect.objectContaining({
          headers: expect.objectContaining({
            'x-requested-with': 'XMLHttpRequest',
          }),
          method: 'POST',
        }),
      );
      expect(response).toBe(payload);
    } finally {
      globalThis.fetch = previousFetch;
    }
  });

  it('reviews an artifact scan status with encoded route params and JSON body', async () => {
    const payload = {
      data: {
        artifact_id: 'artifact/1',
        decision: 'release',
        reviewed: true,
        scan_status: 'clean',
      },
      code: 0,
      msg: '',
    };
    const fetchMock = vi.fn().mockResolvedValue({
      headers: new Headers({
        'content-type': 'application/json',
      }),
      json: () => Promise.resolve(payload),
      ok: true,
    });
    const previousFetch = globalThis.fetch;
    globalThis.fetch = fetchMock;

    try {
      const response = await reviewTaskThreadArtifactScan({
        artifact_id: 'artifact/1',
        decision: 'release',
        reason: 'approved by security reviewer',
        thread_id: 'thread 1',
      });

      expect(fetchMock).toHaveBeenCalledWith(
        '/api/workbench/task_threads/thread%201/artifacts/artifact%2F1/scan_review',
        expect.objectContaining({
          body: JSON.stringify({
            decision: 'release',
            reason: 'approved by security reviewer',
          }),
          headers: expect.objectContaining({
            'content-type': 'application/json',
            'x-requested-with': 'XMLHttpRequest',
          }),
          method: 'POST',
        }),
      );
      expect(response).toBe(payload);
    } finally {
      globalThis.fetch = previousFetch;
    }
  });

  it('fetches artifact content as a blob with encoded route params and mode', async () => {
    const blob = new Blob(['artifact body'], {
      type: 'text/plain; charset=utf-8',
    });
    const fetchMock = vi.fn().mockResolvedValue({
      blob: () => Promise.resolve(blob),
      headers: new Headers({
        'content-disposition': "inline; filename*=UTF-8''report.txt",
        'content-type': 'text/plain; charset=utf-8',
      }),
      ok: true,
    });
    const previousFetch = globalThis.fetch;
    globalThis.fetch = fetchMock;

    try {
      const response = await fetchTaskThreadArtifactContent({
        artifact_id: 'artifact/1',
        mode: 'preview',
        thread_id: 'thread 1',
      });

      expect(fetchMock).toHaveBeenCalledWith(
        '/api/workbench/task_threads/thread%201/artifacts/artifact%2F1/content?mode=preview',
        expect.objectContaining({
          headers: expect.objectContaining({
            'x-requested-with': 'XMLHttpRequest',
          }),
          method: 'GET',
        }),
      );
      expect(response.blob).toBe(blob);
      expect(response.contentDisposition).toBe(
        "inline; filename*=UTF-8''report.txt",
      );
      expect(response.contentType).toBe('text/plain; charset=utf-8');
    } finally {
      globalThis.fetch = previousFetch;
    }
  });

  it('creates an artifact signed URL with encoded route params and TTL', async () => {
    const fetchMock = vi.fn().mockResolvedValue({
      headers: new Headers({
        'content-type': 'application/json',
      }),
      json: () =>
        Promise.resolve({
          code: 0,
          data: {
            artifact_id: 'artifact/1',
            content_type: 'text/plain; charset=utf-8',
            expires_in_seconds: 300,
            preview_mode: 'text',
            url: 'https://storage.example.test/signed/report.txt?token=abc',
          },
          msg: 'success',
        }),
      ok: true,
    });
    const previousFetch = globalThis.fetch;
    globalThis.fetch = fetchMock;

    try {
      const response = await getTaskThreadArtifactSignedURL({
        artifact_id: 'artifact/1',
        mode: 'preview',
        thread_id: 'thread 1',
        ttl_seconds: 300,
      });

      expect(response.data?.url).toContain('https://storage.example.test');
      expect(fetchMock).toHaveBeenCalledWith(
        '/api/workbench/task_threads/thread%201/artifacts/artifact%2F1/signed_url?mode=preview&ttl_seconds=300',
        expect.objectContaining({
          headers: expect.objectContaining({
            'x-requested-with': 'XMLHttpRequest',
          }),
          method: 'GET',
        }),
      );
    } finally {
      globalThis.fetch = previousFetch;
    }
  });

  it('creates an artifact download signed URL with encoded route params', async () => {
    const fetchMock = vi.fn().mockResolvedValue({
      headers: new Headers({
        'content-type': 'application/json',
      }),
      json: () =>
        Promise.resolve({
          code: 0,
          data: {
            artifact_id: 'artifact/1',
            content_type: 'text/html; charset=utf-8',
            expires_in_seconds: 300,
            preview_mode: 'download',
            url: 'https://storage.example.test/signed/page.html?token=abc',
          },
          msg: 'success',
        }),
      ok: true,
    });
    const previousFetch = globalThis.fetch;
    globalThis.fetch = fetchMock;

    try {
      const response = await getTaskThreadArtifactSignedURL({
        artifact_id: 'artifact/1',
        mode: 'download',
        thread_id: 'thread 1',
        ttl_seconds: 300,
      });

      expect(response.data?.url).toContain('https://storage.example.test');
      expect(fetchMock).toHaveBeenCalledWith(
        '/api/workbench/task_threads/thread%201/artifacts/artifact%2F1/signed_url?mode=download&ttl_seconds=300',
        expect.objectContaining({
          headers: expect.objectContaining({
            'x-requested-with': 'XMLHttpRequest',
          }),
          method: 'GET',
        }),
      );
    } finally {
      globalThis.fetch = previousFetch;
    }
  });

  it('maps artifact scan-pending signed URL rejection to safe preview message', async () => {
    const fetchMock = vi.fn().mockResolvedValue({
      headers: new Headers({
        'content-type': 'application/json',
      }),
      json: () =>
        Promise.resolve({
          code: 409,
          msg: 'artifact content blocked by scan policy',
          reason: 'scan_pending',
        }),
      ok: false,
    });
    const previousFetch = globalThis.fetch;
    globalThis.fetch = fetchMock;

    try {
      await expect(
        getTaskThreadArtifactSignedURL({
          artifact_id: 'artifact-pdf',
          mode: 'preview',
          thread_id: 'thread 1',
          ttl_seconds: 300,
        }),
      ).rejects.toThrow('产物安全扫描中，暂不能预览');
      await expect(
        getTaskThreadArtifactSignedURL({
          artifact_id: 'artifact-pdf',
          mode: 'download',
          thread_id: 'thread 1',
          ttl_seconds: 300,
        }),
      ).rejects.toThrow('产物安全扫描中，暂不能下载');
    } finally {
      globalThis.fetch = previousFetch;
    }
  });

  it('deletes a task-thread artifact with encoded route params', async () => {
    const fetchMock = vi.fn().mockResolvedValue({
      headers: new Headers({
        'content-type': 'application/json',
      }),
      json: () => Promise.resolve({ code: 0, msg: 'success' }),
      ok: true,
    });
    const previousFetch = globalThis.fetch;
    globalThis.fetch = fetchMock;

    try {
      await deleteTaskThreadArtifact({
        artifact_id: 'artifact/1',
        thread_id: 'thread 1',
      });

      expect(fetchMock).toHaveBeenCalledWith(
        '/api/workbench/task_threads/thread%201/artifacts/artifact%2F1',
        expect.objectContaining({
          headers: expect.objectContaining({
            'x-requested-with': 'XMLHttpRequest',
          }),
          method: 'DELETE',
        }),
      );
    } finally {
      globalThis.fetch = previousFetch;
    }
  });

  it('restores a task-thread artifact with encoded route params', async () => {
    const fetchMock = vi.fn().mockResolvedValue({
      headers: new Headers({
        'content-type': 'application/json',
      }),
      json: () =>
        Promise.resolve({
          code: 0,
          data: { artifact_id: 'artifact/1', restored: true },
          msg: 'success',
        }),
      ok: true,
    });
    const previousFetch = globalThis.fetch;
    globalThis.fetch = fetchMock;

    try {
      const response = await restoreTaskThreadArtifact({
        artifact_id: 'artifact/1',
        thread_id: 'thread 1',
      });

      expect(response.data?.restored).toBe(true);
      expect(fetchMock).toHaveBeenCalledWith(
        '/api/workbench/task_threads/thread%201/artifacts/artifact%2F1/restore',
        expect.objectContaining({
          headers: expect.objectContaining({
            'x-requested-with': 'XMLHttpRequest',
          }),
          method: 'POST',
        }),
      );
    } finally {
      globalThis.fetch = previousFetch;
    }
  });
});
