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

import { beforeEach, describe, expect, it, vi } from 'vitest';
import {
  workbench,
  workbenchSkill,
  workbenchTask,
} from '@coze-studio/api-schema';

import {
  appendTaskThreadMessage,
  cancelTaskThreadRun,
  clearTaskThreadMemories,
  createTaskThread,
  createTaskThreadRun,
  deleteTaskThreadArtifact,
  deleteTaskThreadMemory,
  exportTaskThreadGuardrailAuditEvents,
  exportTaskThreadMemories,
  fetchTaskThreadArtifactContent,
  generateTaskThreadSuggestions,
  getTaskThread,
  getTaskThreadArtifactSignedURL,
  getTaskThreadTokenUsage,
  getWorkbenchRuntimeDoctor,
  importTaskThreadMemories,
  installSkillFromArtifact,
  listTaskThreadArtifacts,
  listTaskThreadArtifactScanJobs,
  listTaskThreadGuardrailAuditEvents,
  listTaskThreadMemories,
  listTaskThreadMemoryAuditEvents,
  listTaskThreadMessages,
  listTaskThreadMCPRuntimeAuditEvents,
  listTaskThreadRunEvents,
  listTaskThreadRuns,
  listTaskThreads,
  restoreTaskThreadArtifact,
  restoreTaskThreadMemory,
  resumeTaskThreadRun,
  retryTaskThreadArtifactScanJob,
  retryTaskThreadSubagentRun,
  reviewTaskThreadArtifactScan,
  updateTaskThreadMemory,
} from '../service';
import {
  artifactScanJobTransportFixture,
  artifactTransportFixture,
  memoryAuditTransportFixture,
  memoryTransportFixture,
} from '../../workbench/thread-client/__tests__/fixtures';

const canonicalClient = vi.hoisted(() => ({
  contract: 'canonical_v1' as const,
  clearMemories: vi.fn(),
  deleteArtifact: vi.fn(),
  deleteMemory: vi.fn(),
  getArtifactContent: vi.fn(),
  getArtifactSignedURL: vi.fn(),
  listArtifactScanJobs: vi.fn(),
  listMemories: vi.fn(),
  listMemoryAuditEvents: vi.fn(),
  restoreArtifact: vi.fn(),
  restoreMemory: vi.fn(),
  retryArtifactScanJob: vi.fn(),
  reviewArtifactScan: vi.fn(),
  updateMemory: vi.fn(),
}));

const spaceStore = vi.hoisted(() => ({
  getSpaceId: vi.fn(() => 'store-space'),
}));

vi.mock('lottie-web', () => ({
  destroy: vi.fn(),
  loadAnimation: vi.fn(),
  default: { destroy: vi.fn(), loadAnimation: vi.fn() },
}));

vi.mock('../../workbench/thread-client/canonical-thread-client', () => ({
  CanonicalThreadClient: vi.fn(function recordingCanonicalThreadClient() {
    return canonicalClient;
  }),
  CanonicalThreadCoreClient: vi.fn(
    function recordingCanonicalThreadCoreClient() {
      return canonicalClient;
    },
  ),
}));

vi.mock('@coze-foundation/space-store', () => ({
  useSpaceStore: Object.assign(vi.fn(), {
    getState: () => ({ getSpaceId: spaceStore.getSpaceId }),
  }),
}));

const artifact = artifactTransportFixture.visible;
const scanJob = artifactScanJobTransportFixture.visible;
const memory = memoryTransportFixture.visible;
const memoryAudit = memoryAuditTransportFixture.visible;
const blob = new Blob(['artifact'], { type: 'text/plain' });
const directFetch = vi.fn();

beforeEach(() => {
  vi.clearAllMocks();
  vi.stubGlobal('fetch', directFetch);

  canonicalClient.listMemories.mockResolvedValue({
    items: [memory],
    total: 1,
    has_more: false,
  });
  canonicalClient.updateMemory.mockResolvedValue({ memory, updated: true });
  canonicalClient.deleteMemory.mockResolvedValue(undefined);
  canonicalClient.clearMemories.mockResolvedValue({ deleted: 1 });
  canonicalClient.restoreMemory.mockResolvedValue({ memory, restored: true });
  canonicalClient.listMemoryAuditEvents.mockResolvedValue({
    items: [memoryAudit],
    total: 1,
    has_more: false,
  });
  canonicalClient.listArtifactScanJobs.mockResolvedValue({
    items: [scanJob],
    total: 1,
    has_more: false,
  });
  canonicalClient.retryArtifactScanJob.mockResolvedValue({
    job: scanJob,
    retried: true,
  });
  canonicalClient.reviewArtifactScan.mockResolvedValue({
    artifact_id: artifact.artifact_id,
    decision: 'release',
    reviewed: true,
    scan_status: 'clean',
  });
  canonicalClient.getArtifactContent.mockResolvedValue({
    blob,
    content_disposition: 'inline; filename="artifact.txt"',
    content_type: 'text/plain',
  });
  canonicalClient.getArtifactSignedURL.mockResolvedValue({
    artifact_id: artifact.artifact_id,
    content_type: artifact.content_type,
    expires_in_seconds: 300,
    preview_mode: artifact.preview_mode,
    url: 'https://storage.example.test/artifact',
  });
  canonicalClient.deleteArtifact.mockResolvedValue(undefined);
  canonicalClient.restoreArtifact.mockResolvedValue({
    artifact,
    restored: true,
  });
});

describe('task thread service', () => {
  it('keeps every page-facing Task Thread export callable', () => {
    const exports = [
      listTaskThreads,
      createTaskThread,
      getTaskThread,
      listTaskThreadMessages,
      generateTaskThreadSuggestions,
      appendTaskThreadMessage,
      listTaskThreadRuns,
      createTaskThreadRun,
      resumeTaskThreadRun,
      cancelTaskThreadRun,
      retryTaskThreadSubagentRun,
      listTaskThreadRunEvents,
      getTaskThreadTokenUsage,
      listTaskThreadArtifacts,
      listTaskThreadMemories,
      updateTaskThreadMemory,
      deleteTaskThreadMemory,
      clearTaskThreadMemories,
      restoreTaskThreadMemory,
      listTaskThreadMemoryAuditEvents,
      listTaskThreadGuardrailAuditEvents,
      listTaskThreadMCPRuntimeAuditEvents,
      exportTaskThreadMemories,
      exportTaskThreadGuardrailAuditEvents,
      importTaskThreadMemories,
      listTaskThreadArtifactScanJobs,
      retryTaskThreadArtifactScanJob,
      reviewTaskThreadArtifactScan,
      fetchTaskThreadArtifactContent,
      getTaskThreadArtifactSignedURL,
      deleteTaskThreadArtifact,
      restoreTaskThreadArtifact,
    ];

    exports.forEach(serviceExport =>
      expect(serviceExport).toBeTypeOf('function'),
    );
  });

  it('keeps Runtime Doctor and Skill install on generated non-Thread owners', () => {
    expect(getWorkbenchRuntimeDoctor).toBe(workbench.GetWorkbenchRuntimeDoctor);
    expect(getWorkbenchRuntimeDoctor.meta).toMatchObject({
      method: 'GET',
      url: '/api/workbench/runtime_doctor',
    });
    expect(installSkillFromArtifact).toBe(
      workbenchSkill.InstallSkillFromArtifact,
    );
    expect(installSkillFromArtifact.meta).toMatchObject({
      method: 'POST',
      url: '/api/workbench/skills/install',
    });
  });

  it('moves Memory transport away from generated workbenchTask clients', async () => {
    expect(listTaskThreadMemories).not.toBe(
      workbenchTask.ListTaskThreadMemories,
    );
    expect(updateTaskThreadMemory).not.toBe(
      workbenchTask.UpdateTaskThreadMemory,
    );

    const response = await listTaskThreadMemories({
      thread_id: memory.thread_id,
      page: 1,
      page_size: 20,
    });

    expect(response).toEqual({
      code: 0,
      msg: 'success',
      data: { memories: [memory], total: 1 },
    });
    expect(canonicalClient.listMemories).toHaveBeenCalledWith({
      space_id: 'store-space',
      thread_id: memory.thread_id,
      page: 1,
      page_size: 20,
    });
    expect(directFetch).not.toHaveBeenCalled();
  });

  it('delegates Artifact and scan operations with current response shapes', async () => {
    const jobs = await listTaskThreadArtifactScanJobs({
      thread_id: artifact.thread_id,
      artifact_id: artifact.artifact_id,
      page: 2,
      page_size: 10,
    });
    const retry = await retryTaskThreadArtifactScanJob({
      thread_id: artifact.thread_id,
      job_id: scanJob.job_id,
    });
    const review = await reviewTaskThreadArtifactScan({
      thread_id: artifact.thread_id,
      artifact_id: artifact.artifact_id,
      decision: 'release',
      reason: '  reviewed  ',
    });
    const content = await fetchTaskThreadArtifactContent({
      thread_id: artifact.thread_id,
      artifact_id: artifact.artifact_id,
      mode: 'preview',
    });
    const signedURL = await getTaskThreadArtifactSignedURL({
      thread_id: artifact.thread_id,
      artifact_id: artifact.artifact_id,
      mode: 'preview',
      ttl_seconds: 300,
    });
    await deleteTaskThreadArtifact({
      thread_id: artifact.thread_id,
      artifact_id: artifact.artifact_id,
    });
    const restore = await restoreTaskThreadArtifact({
      thread_id: artifact.thread_id,
      artifact_id: artifact.artifact_id,
    });

    expect(jobs).toEqual({
      code: 0,
      msg: 'success',
      data: { jobs: [scanJob], total: 1 },
    });
    expect(retry).toMatchObject({
      code: 0,
      msg: 'success',
      data: { retried: true },
    });
    expect(review).toMatchObject({ code: 0, msg: 'success' });
    expect(content).toEqual({
      blob,
      contentDisposition: 'inline; filename="artifact.txt"',
      contentType: 'text/plain',
    });
    expect(signedURL.data?.url).toBe('https://storage.example.test/artifact');
    expect(restore).toEqual({
      code: 0,
      msg: 'success',
      data: { artifact_id: artifact.artifact_id, restored: true },
    });
    expect(canonicalClient.reviewArtifactScan).toHaveBeenCalledWith({
      artifact_id: artifact.artifact_id,
      decision: 'release',
      reason: '  reviewed  ',
      space_id: 'store-space',
      thread_id: artifact.thread_id,
    });
    expect(directFetch).not.toHaveBeenCalled();
  });

  it('maps canonical scan policy codes to safe page errors', async () => {
    canonicalClient.getArtifactSignedURL.mockRejectedValue(
      Object.assign(new Error('internal detail must not escape'), {
        code: 'scan_pending',
      }),
    );

    await expect(
      getTaskThreadArtifactSignedURL({
        thread_id: artifact.thread_id,
        artifact_id: artifact.artifact_id,
        mode: 'preview',
      }),
    ).rejects.toThrow('产物安全扫描中，暂不能预览');
    await expect(
      getTaskThreadArtifactSignedURL({
        thread_id: artifact.thread_id,
        artifact_id: artifact.artifact_id,
        mode: 'download',
      }),
    ).rejects.toThrow('产物安全扫描中，暂不能下载');
  });
});
