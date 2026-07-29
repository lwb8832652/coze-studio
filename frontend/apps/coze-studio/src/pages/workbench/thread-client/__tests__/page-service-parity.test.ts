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

import * as workbenchService from '../../service';
import * as taskMemoryService from '../../../tasks/task-memory-service';
import * as taskService from '../../../tasks/service';
import {
  artifactScanJobTransportFixture,
  artifactTransportFixture,
  guardrailAuditTransportFixture,
  mcpRuntimeAuditTransportFixture,
  memoryAuditTransportFixture,
  memoryTransportFixture,
  messageTransportFixture,
  runEventTransportFixture,
  runTransportFixture,
  threadTransportFixture,
  tokenUsageTransportFixture,
  uploadTransportFixture,
} from './fixtures';

const recordingCanonicalClient = vi.hoisted(() => ({
  contract: 'canonical_v1' as const,
  appendMessage: vi.fn(),
  cancelRun: vi.fn(),
  clearMemories: vi.fn(),
  createRun: vi.fn(),
  createThread: vi.fn(),
  deleteArtifact: vi.fn(),
  deleteMemory: vi.fn(),
  deleteUpload: vi.fn(),
  exportGuardrailAuditEvents: vi.fn(),
  exportMemories: vi.fn(),
  generateSuggestions: vi.fn(),
  getArtifactContent: vi.fn(),
  getArtifactSignedURL: vi.fn(),
  getRun: vi.fn(),
  getThread: vi.fn(),
  getTokenUsage: vi.fn(),
  importMemories: vi.fn(),
  listArtifactScanJobs: vi.fn(),
  listArtifacts: vi.fn(),
  listGuardrailAuditEvents: vi.fn(),
  listMCPRuntimeAuditEvents: vi.fn(),
  listMemories: vi.fn(),
  listMemoryAuditEvents: vi.fn(),
  listMessages: vi.fn(),
  listRunEvents: vi.fn(),
  listRuns: vi.fn(),
  listUploads: vi.fn(),
  restoreArtifact: vi.fn(),
  restoreMemory: vi.fn(),
  resumeRun: vi.fn(),
  retryArtifactScanJob: vi.fn(),
  retrySubagentRun: vi.fn(),
  reviewArtifactScan: vi.fn(),
  searchThreads: vi.fn(),
  subscribeRunEvents: vi.fn(),
  updateMemory: vi.fn(),
  uploadFiles: vi.fn(),
}));

const spaceStore = vi.hoisted(() => ({
  currentSpaceID: 'store-space-1',
  getSpaceId: vi.fn(() => spaceStore.currentSpaceID),
}));

const generatedOwners = vi.hoisted(() => {
  const method = () => vi.fn();
  const tokenUsage = Object.assign(method(), {
    withAbort: vi.fn(() => Object.assign(method(), { abort: vi.fn() })),
  });

  return {
    developerModels: method(),
    installSkillFromArtifact: method(),
    knowledge: method(),
    database: method(),
    runtimeDoctor: method(),
    tokenUsage,
    workflow: method(),
    workspaceModels: method(),
    task: {
      AppendTaskThreadMessage: method(),
      CancelTaskThreadRun: method(),
      ClearTaskThreadMemories: method(),
      CreateTaskThread: method(),
      CreateTaskThreadRun: method(),
      DeleteTaskThreadMemory: method(),
      ExportTaskThreadGuardrailAuditEvents: method(),
      ExportTaskThreadMemories: method(),
      GenerateTaskThreadSuggestions: method(),
      GetTaskThread: method(),
      GetTaskThreadTokenUsage: tokenUsage,
      ImportTaskThreadMemories: method(),
      ListTaskThreadArtifacts: method(),
      ListTaskThreadGuardrailAuditEvents: method(),
      ListTaskThreadMCPRuntimeAuditEvents: method(),
      ListTaskThreadMemories: method(),
      ListTaskThreadMemoryAuditEvents: method(),
      ListTaskThreadMessages: method(),
      ListTaskThreadRunEvents: method(),
      ListTaskThreadRuns: method(),
      ListTaskThreads: method(),
      RestoreTaskThreadMemory: method(),
      ResumeTaskThreadRun: method(),
      RetryTaskThreadSubagentRun: method(),
      UpdateTaskThreadMemory: method(),
    },
  };
});

vi.mock('lottie-web', () => ({
  default: { destroy: vi.fn(), loadAnimation: vi.fn() },
  destroy: vi.fn(),
  loadAnimation: vi.fn(),
}));

vi.mock('../canonical-thread-client', () => ({
  CanonicalThreadClient: vi.fn(function recordingCanonicalThreadClient() {
    return recordingCanonicalClient;
  }),
  CanonicalThreadCoreClient: vi.fn(
    function recordingCanonicalThreadCoreClient() {
      return recordingCanonicalClient;
    },
  ),
}));

vi.mock('@coze-foundation/space-store', () => ({
  useSpaceStore: Object.assign(vi.fn(), {
    getState: () => ({ getSpaceId: spaceStore.getSpaceId }),
  }),
}));

vi.mock('@coze-studio/api-schema', () => ({
  workbench: {
    GetWorkbenchRuntimeDoctor: generatedOwners.runtimeDoctor,
  },
  workbenchSkill: {
    InstallSkillFromArtifact: generatedOwners.installSkillFromArtifact,
  },
  workbenchTask: generatedOwners.task,
}));

vi.mock('@coze-studio/api-schema/workbench-model', () => ({
  ListWorkspaceModels: generatedOwners.workspaceModels,
  WorkspaceModelScope: { Space: 1 },
}));

vi.mock('@coze-arch/bot-api', () => ({
  DeveloperApi: { GetTypeList: generatedOwners.developerModels },
  KnowledgeApi: { ListDataset: generatedOwners.knowledge },
  MemoryApi: { ListDatabase: generatedOwners.database },
  workflowApi: { WorkflowListV2: generatedOwners.workflow },
}));

const success = <T>(data: T) => ({ code: 0, msg: 'success', data });

const thread = threadTransportFixture.visible;
const message = messageTransportFixture.visible;
const run = runTransportFixture.visible;
const runEvent = runEventTransportFixture.visible;
const upload = uploadTransportFixture.visible;
const artifact = artifactTransportFixture.visible;
const scanJob = artifactScanJobTransportFixture.visible;
const usage = tokenUsageTransportFixture.visible;
const memory = memoryTransportFixture.visible;
const memoryAudit = memoryAuditTransportFixture.visible;
const guardrailAudit = guardrailAuditTransportFixture.visible;
const mcpAudit = mcpRuntimeAuditTransportFixture.visible;
const artifactBlob = new Blob(['canonical artifact'], { type: 'text/plain' });

const sourceFetch = vi.fn();

const setupCanonicalResponses = () => {
  recordingCanonicalClient.searchThreads.mockResolvedValue({
    items: [thread],
    total: 1,
    has_more: false,
  });
  recordingCanonicalClient.createThread.mockResolvedValue({
    thread,
    message,
    run,
  });
  recordingCanonicalClient.getThread.mockResolvedValue(thread);
  recordingCanonicalClient.listMessages.mockResolvedValue({
    items: [message],
    total: 1,
    has_more: false,
  });
  recordingCanonicalClient.appendMessage.mockResolvedValue(message);
  recordingCanonicalClient.generateSuggestions.mockResolvedValue([
    'Continue the brief',
  ]);
  recordingCanonicalClient.listRuns.mockResolvedValue({
    items: [run],
    total: 1,
    has_more: false,
  });
  recordingCanonicalClient.createRun.mockResolvedValue({ run, message });
  recordingCanonicalClient.getRun.mockResolvedValue(run);
  recordingCanonicalClient.cancelRun.mockResolvedValue(undefined);
  recordingCanonicalClient.resumeRun.mockResolvedValue(run);
  recordingCanonicalClient.retrySubagentRun.mockResolvedValue(run);
  recordingCanonicalClient.listRunEvents.mockResolvedValue({
    items: [runEvent],
    total: 1,
    has_more: false,
  });
  recordingCanonicalClient.uploadFiles.mockResolvedValue({
    uploads: [upload],
    skipped_files: ['ignored.tmp'],
  });
  recordingCanonicalClient.listArtifacts.mockResolvedValue({
    items: [artifact],
    total: 1,
    has_more: false,
  });
  recordingCanonicalClient.getArtifactContent.mockResolvedValue({
    blob: artifactBlob,
    content_disposition: 'inline; filename="artifact.txt"',
    content_type: 'text/plain',
  });
  recordingCanonicalClient.getArtifactSignedURL.mockResolvedValue({
    artifact_id: artifact.artifact_id,
    content_type: artifact.content_type,
    expires_in_seconds: 300,
    preview_mode: artifact.preview_mode,
    url: 'https://storage.example.test/artifact',
  });
  recordingCanonicalClient.deleteArtifact.mockResolvedValue(undefined);
  recordingCanonicalClient.restoreArtifact.mockResolvedValue({
    artifact,
    restored: true,
  });
  recordingCanonicalClient.reviewArtifactScan.mockResolvedValue({
    artifact_id: artifact.artifact_id,
    decision: 'release',
    reviewed: true,
    scan_status: 'clean',
  });
  recordingCanonicalClient.listArtifactScanJobs.mockResolvedValue({
    items: [scanJob],
    total: 1,
    has_more: false,
  });
  recordingCanonicalClient.retryArtifactScanJob.mockResolvedValue({
    job: scanJob,
    retried: true,
  });
  recordingCanonicalClient.getTokenUsage.mockResolvedValue(usage);
  recordingCanonicalClient.listMemories.mockResolvedValue({
    items: [memory],
    total: 1,
    has_more: false,
  });
  recordingCanonicalClient.updateMemory.mockResolvedValue({
    memory,
    updated: true,
  });
  recordingCanonicalClient.deleteMemory.mockResolvedValue(undefined);
  recordingCanonicalClient.clearMemories.mockResolvedValue({ deleted: 2 });
  recordingCanonicalClient.restoreMemory.mockResolvedValue({
    memory,
    restored: true,
  });
  recordingCanonicalClient.importMemories.mockResolvedValue({
    imported: 1,
    skipped: 0,
    memories: [memory],
  });
  recordingCanonicalClient.exportMemories.mockResolvedValue({
    schema: 'coze.task_thread_memories.v1',
    thread_id: thread.thread_id,
    exported_at: thread.updated_at,
    total: 1,
    memories: [memory],
  });
  recordingCanonicalClient.listMemoryAuditEvents.mockResolvedValue({
    items: [memoryAudit],
    total: 1,
    has_more: false,
  });
  recordingCanonicalClient.listGuardrailAuditEvents.mockResolvedValue({
    items: [guardrailAudit],
    total: 1,
    has_more: false,
  });
  recordingCanonicalClient.exportGuardrailAuditEvents.mockResolvedValue({
    schema: 'coze.guardrail_audit.v1',
    thread_id: thread.thread_id,
    exported_at: thread.updated_at,
    total: 1,
    events: [guardrailAudit],
  });
  recordingCanonicalClient.listMCPRuntimeAuditEvents.mockResolvedValue({
    items: [mcpAudit],
    total: 1,
    has_more: false,
  });
};

const setupGeneratedSourceResponses = () => {
  const { task } = generatedOwners;
  task.ListTaskThreads.mockResolvedValue(
    success({ threads: [thread], total: 1 }),
  );
  task.CreateTaskThread.mockResolvedValue(success({ thread, message, run }));
  task.GetTaskThread.mockResolvedValue(success(thread));
  task.ListTaskThreadMessages.mockResolvedValue(
    success({ messages: [message], total: 1 }),
  );
  task.AppendTaskThreadMessage.mockResolvedValue(success(message));
  task.GenerateTaskThreadSuggestions.mockResolvedValue({
    suggestions: ['Continue the brief'],
  });
  task.ListTaskThreadRuns.mockResolvedValue(success({ runs: [run], total: 1 }));
  task.CreateTaskThreadRun.mockResolvedValue({
    code: 0,
    msg: 'success',
    data: run,
    message,
  });
  task.ResumeTaskThreadRun.mockResolvedValue(success(run));
  task.CancelTaskThreadRun.mockResolvedValue(success(run));
  task.RetryTaskThreadSubagentRun.mockResolvedValue(success(run));
  task.ListTaskThreadRunEvents.mockResolvedValue(
    success({ events: [runEvent], total: 1 }),
  );
  task.ListTaskThreadArtifacts.mockResolvedValue(
    success({ artifacts: [artifact], total: 1 }),
  );
  task.ListTaskThreadMemories.mockResolvedValue(
    success({ memories: [memory], total: 1 }),
  );
  task.UpdateTaskThreadMemory.mockResolvedValue(
    success({ memory, updated: true }),
  );
  task.DeleteTaskThreadMemory.mockResolvedValue({ code: 0, msg: 'success' });
  task.ClearTaskThreadMemories.mockResolvedValue(success({ deleted: 2 }));
  task.RestoreTaskThreadMemory.mockResolvedValue(
    success({ memory, restored: true }),
  );
  task.ListTaskThreadMemoryAuditEvents.mockResolvedValue(
    success({ events: [memoryAudit], total: 1 }),
  );
  task.ListTaskThreadGuardrailAuditEvents.mockResolvedValue(
    success({ events: [guardrailAudit], total: 1 }),
  );
  task.ListTaskThreadMCPRuntimeAuditEvents.mockResolvedValue(
    success({ events: [mcpAudit], total: 1 }),
  );
  task.ExportTaskThreadMemories.mockResolvedValue(
    success({
      schema: 'coze.task_thread_memories.v1',
      thread_id: thread.thread_id,
      exported_at: thread.updated_at,
      total: 1,
      memories: [memory],
    }),
  );
  task.ExportTaskThreadGuardrailAuditEvents.mockResolvedValue(
    success({
      schema: 'coze.guardrail_audit.v1',
      thread_id: thread.thread_id,
      exported_at: thread.updated_at,
      page: 2,
      page_size: 10,
      total: 1,
      events: [guardrailAudit],
    }),
  );
  task.ImportTaskThreadMemories.mockResolvedValue(
    success({ imported: 1, skipped: 0, memories: [memory] }),
  );

  generatedOwners.tokenUsage.withAbort.mockReturnValue(
    Object.assign(
      vi.fn().mockResolvedValue(
        success({
          usage: usage.items,
          total: usage.total,
          aggregate: usage.aggregate,
          run_aggregates: usage.run_aggregates,
        }),
      ),
      { abort: vi.fn() },
    ),
  );
};

beforeEach(() => {
  vi.clearAllMocks();
  spaceStore.currentSpaceID = 'store-space-1';
  setupCanonicalResponses();
  setupGeneratedSourceResponses();
  sourceFetch.mockClear();
  vi.stubGlobal('fetch', sourceFetch);

  generatedOwners.runtimeDoctor.mockResolvedValue(
    success({ status: 'healthy' }),
  );
  generatedOwners.installSkillFromArtifact.mockResolvedValue(
    success({ installed: true }),
  );
  generatedOwners.developerModels.mockResolvedValue({
    data: {
      model_list: [{ model_name: 'model-a', model_brief_desc: 'legacy' }],
    },
  });
  generatedOwners.workspaceModels.mockResolvedValue(
    success({
      can_manage: true,
      workspace_models: [
        {
          id: 'workspace-model-a',
          model_identifier: 'model-a',
          can_manage: true,
          description: 'workspace model',
        },
      ],
    }),
  );
  generatedOwners.knowledge.mockResolvedValue({
    dataset_list: [{ dataset_id: 'kb-1', name: 'Knowledge' }],
  });
  generatedOwners.database.mockResolvedValue({
    database_info_list: [{ id: 'db-1', table_name: 'Database' }],
  });
  generatedOwners.workflow.mockResolvedValue({
    data: { workflow_list: [{ workflow_id: 'wf-1', name: 'Workflow' }] },
  });
});

describe('page service canonical delegation parity', () => {
  it('delegates Workbench Thread writes and uploads while preserving envelopes', async () => {
    const createResponse = await workbenchService.createTaskThread({
      space_id: 'explicit-space',
      message: 'Prepare the launch brief',
      defer_start: true,
    });
    const appendResponse = await workbenchService.appendTaskThreadMessage({
      thread_id: thread.thread_id,
      run_id: run.run_id,
      role: 'assistant',
      content: 'Metrics added',
    });
    const runResponse = await workbenchService.createTaskThreadRun({
      thread_id: thread.thread_id,
      input: '{"message":"Add metrics"}',
      message_content: 'Add metrics',
    });
    const uploadResponse = await workbenchService.uploadTaskThreadFiles({
      thread_id: thread.thread_id,
      files: [new File(['brief'], 'brief.md')],
    });

    expect(createResponse).toEqual(success({ thread, message, run }));
    expect(appendResponse).toEqual(success(message));
    expect(runResponse).toEqual({
      code: 0,
      msg: 'success',
      data: run,
      message,
    });
    expect(uploadResponse).toEqual(
      success({ files: [upload], skipped_files: ['ignored.tmp'] }),
    );
    expect(recordingCanonicalClient.createThread).toHaveBeenCalledWith({
      space_id: 'explicit-space',
      message: 'Prepare the launch brief',
      defer_start: true,
    });
    expect(recordingCanonicalClient.appendMessage).toHaveBeenCalledWith({
      space_id: 'store-space-1',
      thread_id: thread.thread_id,
      run_id: run.run_id,
      role: 'assistant',
      content: 'Metrics added',
    });
    expect(recordingCanonicalClient.createRun).toHaveBeenCalledWith({
      space_id: 'store-space-1',
      thread_id: thread.thread_id,
      input: '{"message":"Add metrics"}',
      message_content: 'Add metrics',
    });
    expect(recordingCanonicalClient.uploadFiles).toHaveBeenCalledWith(
      expect.objectContaining({
        space_id: 'store-space-1',
        thread_id: thread.thread_id,
      }),
    );
    expect(spaceStore.getSpaceId).toHaveBeenCalledTimes(3);
    expect(sourceFetch).not.toHaveBeenCalled();
  });

  it('delegates core Tasks services and resolves store space on every call', async () => {
    const listResponse = await taskService.listTaskThreads({
      space_id: 'explicit-space',
      page: 1,
      page_size: 20,
    });
    const createResponse = await taskService.createTaskThread({
      space_id: 'explicit-space',
      message: 'Prepare the launch brief',
    });
    const firstThreadResponse = await taskService.getTaskThread({
      thread_id: thread.thread_id,
    });
    spaceStore.currentSpaceID = 'store-space-2';
    const secondThreadResponse = await taskService.getTaskThread({
      thread_id: thread.thread_id,
    });
    const messageResponse = await taskService.listTaskThreadMessages({
      thread_id: thread.thread_id,
      page: 1,
      page_size: 50,
    });
    const suggestionResponse = await taskService.generateTaskThreadSuggestions({
      thread_id: thread.thread_id,
      messages: [{ role: 'user', content: 'Next?' }],
      n: 1,
    });
    const runsResponse = await taskService.listTaskThreadRuns({
      thread_id: thread.thread_id,
      parent_run_id: '0',
      page: 1,
      page_size: 1,
    });
    const appendResponse = await taskService.appendTaskThreadMessage({
      thread_id: thread.thread_id,
      run_id: run.run_id,
      role: 'assistant',
      content: 'Next step prepared',
    });
    const createRunResponse = await taskService.createTaskThreadRun({
      thread_id: thread.thread_id,
      input: '{"message":"Next?"}',
    });
    const resumeResponse = await taskService.resumeTaskThreadRun({
      thread_id: thread.thread_id,
      run_id: run.run_id,
      interrupt_id: '10001',
      response: {
        schema: 'coze.human_interaction_response.v1',
        interaction_id: '10001',
        kind: 'confirmation',
        decision: 'approved',
      },
    });
    const cancelResponse = await taskService.cancelTaskThreadRun({
      thread_id: thread.thread_id,
      run_id: run.run_id,
    });
    const retryResponse = await taskService.retryTaskThreadSubagentRun({
      thread_id: thread.thread_id,
      run_id: run.run_id,
    });
    const artifactsResponse = await taskService.listTaskThreadArtifacts({
      thread_id: thread.thread_id,
      space_id: 'artifact-space',
      page: 1,
      page_size: 20,
    });

    expect(listResponse).toEqual(success({ threads: [thread], total: 1 }));
    expect(createResponse).toEqual(success({ thread, message, run }));
    expect(firstThreadResponse).toEqual(success(thread));
    expect(secondThreadResponse).toEqual(success(thread));
    expect(messageResponse).toEqual(success({ messages: [message], total: 1 }));
    expect(suggestionResponse).toEqual({ suggestions: ['Continue the brief'] });
    expect(runsResponse).toEqual(success({ runs: [run], total: 1 }));
    expect(appendResponse).toEqual(success(message));
    expect(createRunResponse).toEqual({
      code: 0,
      msg: 'success',
      data: run,
      message,
    });
    expect(resumeResponse).toEqual(success(run));
    expect(cancelResponse).toEqual({ code: 0, msg: 'success' });
    expect(retryResponse).toEqual(success(run));
    expect(artifactsResponse).toEqual(
      success({ artifacts: [artifact], total: 1 }),
    );
    expect(recordingCanonicalClient.searchThreads).toHaveBeenCalledWith({
      space_id: 'explicit-space',
      page: 1,
      page_size: 20,
    });
    expect(recordingCanonicalClient.createThread).toHaveBeenCalledWith({
      space_id: 'explicit-space',
      message: 'Prepare the launch brief',
    });
    expect(recordingCanonicalClient.getThread).toHaveBeenNthCalledWith(1, {
      space_id: 'store-space-1',
      thread_id: thread.thread_id,
    });
    expect(recordingCanonicalClient.getThread).toHaveBeenNthCalledWith(2, {
      space_id: 'store-space-2',
      thread_id: thread.thread_id,
    });
    expect(recordingCanonicalClient.listRuns).toHaveBeenCalledWith({
      space_id: 'store-space-2',
      thread_id: thread.thread_id,
      page: 1,
      page_size: 1,
    });
    expect(recordingCanonicalClient.listMessages).toHaveBeenCalledWith({
      space_id: 'store-space-2',
      thread_id: thread.thread_id,
      limit: 50,
    });
    expect(recordingCanonicalClient.listArtifacts).toHaveBeenCalledWith({
      space_id: 'artifact-space',
      thread_id: thread.thread_id,
      page: 1,
      page_size: 20,
    });
    expect(recordingCanonicalClient.getRun).not.toHaveBeenCalled();
  });

  it('does not replace an explicitly empty space with the current store space', async () => {
    await taskService.getTaskThread({
      thread_id: thread.thread_id,
      space_id: '',
    });

    expect(recordingCanonicalClient.getThread).toHaveBeenCalledWith({
      thread_id: thread.thread_id,
      space_id: '',
    });
    expect(spaceStore.getSpaceId).not.toHaveBeenCalled();
  });

  it('maps legacy Message pages to the canonical sequence cursor', async () => {
    recordingCanonicalClient.listMessages.mockResolvedValueOnce({
      items: [message],
      total: 51,
      has_more: false,
    });

    const response = await taskService.listTaskThreadMessages({
      thread_id: thread.thread_id,
      page: 2,
      page_size: 50,
    });

    expect(response).toEqual(success({ messages: [message], total: 51 }));
    expect(recordingCanonicalClient.listMessages).toHaveBeenCalledWith({
      space_id: 'store-space-1',
      thread_id: thread.thread_id,
      limit: 50,
      after_seq: '50',
    });
  });

  it('preserves the legacy Message total when the canonical page has more data', async () => {
    recordingCanonicalClient.listMessages.mockResolvedValueOnce({
      items: [message],
      total: 120,
      has_more: true,
      next_after_seq: '50',
    });

    const response = await taskService.listTaskThreadMessages({
      thread_id: thread.thread_id,
      page: 1,
      page_size: 50,
    });

    expect(response).toEqual(success({ messages: [message], total: 120 }));
    expect(recordingCanonicalClient.listMessages).toHaveBeenCalledTimes(1);
  });

  it('bridges missing run_id before listing events and preserves optional journal shape', async () => {
    const response = await taskService.listTaskThreadRunEvents({
      thread_id: thread.thread_id,
      page: 1,
      page_size: 100,
    });

    expect(recordingCanonicalClient.listRuns).toHaveBeenCalledWith({
      space_id: 'store-space-1',
      thread_id: thread.thread_id,
      page: 1,
      page_size: 1,
    });
    expect(recordingCanonicalClient.listRunEvents).toHaveBeenCalledWith({
      space_id: 'store-space-1',
      thread_id: thread.thread_id,
      run_id: run.run_id,
      limit: 100,
    });
    expect(response).toEqual(success({ events: [runEvent], total: 1 }));
    expect(response.data).not.toHaveProperty('journal_messages');

    recordingCanonicalClient.listRuns.mockResolvedValueOnce({
      items: [],
      total: 0,
      has_more: false,
    });
    recordingCanonicalClient.listRunEvents.mockClear();
    const emptyResponse = await taskService.listTaskThreadRunEvents({
      thread_id: thread.thread_id,
      page: 1,
      page_size: 100,
    });

    expect(emptyResponse).toEqual(success({ events: [], total: 0 }));
    expect(emptyResponse.data).not.toHaveProperty('journal_messages');
    expect(recordingCanonicalClient.listRunEvents).not.toHaveBeenCalled();

    recordingCanonicalClient.listRuns.mockClear();
    const explicitRunResponse = await taskService.listTaskThreadRunEvents({
      thread_id: thread.thread_id,
      run_id: run.run_id,
      page: 1,
      page_size: 25,
    });

    expect(explicitRunResponse).toEqual(
      success({ events: [runEvent], total: 1 }),
    );
    expect(recordingCanonicalClient.listRuns).not.toHaveBeenCalled();
    expect(recordingCanonicalClient.listRunEvents).toHaveBeenCalledWith({
      space_id: 'store-space-1',
      thread_id: thread.thread_id,
      run_id: run.run_id,
      limit: 25,
    });
  });

  it('preserves the legacy Run Event total while returning the requested page', async () => {
    recordingCanonicalClient.listRunEvents.mockResolvedValueOnce({
      items: [runEvent],
      total: 2,
      has_more: true,
      next_cursor: runEvent.event_id,
    });

    const response = await taskService.listTaskThreadRunEvents({
      thread_id: thread.thread_id,
      run_id: run.run_id,
      page: 1,
      page_size: 25,
    });

    expect(response).toEqual(success({ events: [runEvent], total: 2 }));
    expect(recordingCanonicalClient.listRunEvents).toHaveBeenCalledTimes(1);
  });

  it('walks canonical Run Event cursors for legacy pages after the first', async () => {
    recordingCanonicalClient.listRunEvents
      .mockResolvedValueOnce({
        items: [runEvent],
        total: 2,
        has_more: true,
        next_cursor: runEvent.event_id,
      })
      .mockResolvedValueOnce({
        items: [runEvent],
        total: 2,
        has_more: false,
      });

    const response = await taskService.listTaskThreadRunEvents({
      thread_id: thread.thread_id,
      run_id: run.run_id,
      page: 2,
      page_size: 25,
    });

    expect(response).toEqual(success({ events: [runEvent], total: 2 }));
    expect(recordingCanonicalClient.listRunEvents).toHaveBeenNthCalledWith(1, {
      space_id: 'store-space-1',
      thread_id: thread.thread_id,
      run_id: run.run_id,
      limit: 25,
    });
    expect(recordingCanonicalClient.listRunEvents).toHaveBeenNthCalledWith(2, {
      space_id: 'store-space-1',
      thread_id: thread.thread_id,
      run_id: run.run_id,
      cursor: runEvent.event_id,
      limit: 25,
    });
  });

  it('delegates Artifact and scan operations while preserving Blob headers', async () => {
    const scanList = await taskService.listTaskThreadArtifactScanJobs({
      thread_id: thread.thread_id,
      space_id: 'artifact-space',
      artifact_id: artifact.artifact_id,
      page: 2,
      page_size: 10,
    });
    const scanRetry = await taskService.retryTaskThreadArtifactScanJob({
      thread_id: thread.thread_id,
      job_id: scanJob.job_id,
    });
    const scanReview = await taskService.reviewTaskThreadArtifactScan({
      thread_id: thread.thread_id,
      artifact_id: artifact.artifact_id,
      decision: 'release',
      reason: '  approved  ',
    });
    const content = await taskService.fetchTaskThreadArtifactContent({
      thread_id: thread.thread_id,
      artifact_id: artifact.artifact_id,
      mode: 'preview',
    });
    const signedURL = await taskService.getTaskThreadArtifactSignedURL({
      thread_id: thread.thread_id,
      artifact_id: artifact.artifact_id,
      mode: 'preview',
      ttl_seconds: 300,
    });
    const deleted = await taskService.deleteTaskThreadArtifact({
      thread_id: thread.thread_id,
      artifact_id: artifact.artifact_id,
    });
    const restored = await taskService.restoreTaskThreadArtifact({
      thread_id: thread.thread_id,
      artifact_id: artifact.artifact_id,
    });

    expect(scanList).toEqual(success({ jobs: [scanJob], total: 1 }));
    expect(scanRetry).toEqual(success({ job: scanJob, retried: true }));
    expect(scanReview).toEqual(
      success({
        artifact_id: artifact.artifact_id,
        decision: 'release',
        reviewed: true,
        scan_status: 'clean',
      }),
    );
    expect(content).toEqual({
      blob: artifactBlob,
      contentDisposition: 'inline; filename="artifact.txt"',
      contentType: 'text/plain',
    });
    expect(signedURL).toEqual(
      success({
        artifact_id: artifact.artifact_id,
        content_type: artifact.content_type,
        expires_in_seconds: 300,
        preview_mode: artifact.preview_mode,
        url: 'https://storage.example.test/artifact',
      }),
    );
    expect(deleted).toBeUndefined();
    expect(restored).toEqual(
      success({ artifact_id: artifact.artifact_id, restored: true }),
    );
    expect(recordingCanonicalClient.reviewArtifactScan).toHaveBeenCalledWith({
      space_id: 'store-space-1',
      thread_id: thread.thread_id,
      artifact_id: artifact.artifact_id,
      decision: 'release',
      reason: '  approved  ',
    });
    expect(sourceFetch).not.toHaveBeenCalled();
  });

  it('keeps safe Artifact scan errors at the page boundary', async () => {
    const scanBlocked = Object.assign(new Error('internal detail'), {
      code: 'artifact_content_blocked',
    });
    recordingCanonicalClient.getArtifactSignedURL.mockRejectedValue(
      scanBlocked,
    );

    await expect(
      taskService.getTaskThreadArtifactSignedURL({
        thread_id: thread.thread_id,
        artifact_id: artifact.artifact_id,
        mode: 'preview',
      }),
    ).rejects.toThrow('产物受安全策略限制，暂不能预览');
    await expect(
      taskService.getTaskThreadArtifactSignedURL({
        thread_id: thread.thread_id,
        artifact_id: artifact.artifact_id,
        mode: 'download',
      }),
    ).rejects.toThrow('产物受安全策略限制，暂不能下载');
  });

  it('delegates Memory and audit families from both service modules', async () => {
    const listRequest = {
      thread_id: thread.thread_id,
      q: '  launch  ',
      scopes: ['thread'],
      page: 1,
      page_size: 20,
    };
    const updateRequest = {
      thread_id: thread.thread_id,
      memory_id: memory.memory_id,
      scope: memory.scope,
      content: memory.content,
      confidence: 0,
      corrected_at: 0,
      expires_at: 0,
      metadata: '',
      score: 0,
    };

    const taskResponses = await Promise.all([
      taskService.listTaskThreadMemories(listRequest),
      taskService.updateTaskThreadMemory(updateRequest),
      taskService.deleteTaskThreadMemory({
        thread_id: thread.thread_id,
        memory_id: memory.memory_id,
      }),
      taskService.clearTaskThreadMemories({
        thread_id: thread.thread_id,
        scopes: [],
      }),
      taskService.restoreTaskThreadMemory({
        thread_id: thread.thread_id,
        memory_id: memory.memory_id,
      }),
      taskService.listTaskThreadMemoryAuditEvents({
        thread_id: thread.thread_id,
        memory_id: memory.memory_id,
        page: 1,
        page_size: 20,
      }),
      taskService.listTaskThreadGuardrailAuditEvents({
        thread_id: thread.thread_id,
        run_id: run.run_id,
        page: 1,
        page_size: 20,
      }),
      taskService.listTaskThreadMCPRuntimeAuditEvents({
        thread_id: thread.thread_id,
        run_id: run.run_id,
        page: 1,
        page_size: 20,
      }),
      taskService.exportTaskThreadMemories({
        thread_id: thread.thread_id,
        limit: 100,
      }),
      taskService.exportTaskThreadGuardrailAuditEvents({
        thread_id: thread.thread_id,
        page: 2,
        page_size: 10,
      }),
      taskService.importTaskThreadMemories({
        thread_id: thread.thread_id,
        memories: [{ content: memory.content, scope: memory.scope }],
      }),
    ]);
    const legacyModuleResponses = await Promise.all([
      taskMemoryService.listTaskThreadMemories(listRequest),
      taskMemoryService.updateTaskThreadMemory(updateRequest),
      taskMemoryService.deleteTaskThreadMemory({
        thread_id: thread.thread_id,
        memory_id: memory.memory_id,
      }),
      taskMemoryService.clearTaskThreadMemories({
        thread_id: thread.thread_id,
        scopes: [],
      }),
      taskMemoryService.restoreTaskThreadMemory({
        thread_id: thread.thread_id,
        memory_id: memory.memory_id,
      }),
      taskMemoryService.listTaskThreadMemoryAuditEvents({
        thread_id: thread.thread_id,
        memory_id: memory.memory_id,
        page: 1,
        page_size: 20,
      }),
    ]);

    expect(taskResponses[0]).toEqual(success({ memories: [memory], total: 1 }));
    expect(taskResponses[1]).toEqual(success({ memory, updated: true }));
    expect(taskResponses[2]).toEqual({ code: 0, msg: 'success' });
    expect(taskResponses[3]).toEqual(success({ deleted: 2 }));
    expect(taskResponses[4]).toEqual(success({ memory, restored: true }));
    expect(taskResponses[5]).toEqual(
      success({ events: [memoryAudit], total: 1 }),
    );
    expect(taskResponses[6]).toEqual(
      success({ events: [guardrailAudit], total: 1 }),
    );
    expect(taskResponses[7]).toEqual(success({ events: [mcpAudit], total: 1 }));
    expect(taskResponses[8]).toMatchObject({
      code: 0,
      msg: 'success',
      data: { memories: [memory], total: 1 },
    });
    expect(taskResponses[9]).toEqual(
      success({
        schema: 'coze.guardrail_audit.v1',
        thread_id: thread.thread_id,
        exported_at: thread.updated_at,
        page: 2,
        page_size: 10,
        total: 1,
        events: [guardrailAudit],
      }),
    );
    expect(taskResponses[10]).toEqual(
      success({ imported: 1, skipped: 0, memories: [memory] }),
    );
    expect(legacyModuleResponses).toEqual([
      success({ memories: [memory], total: 1 }),
      success({ memory, updated: true }),
      { code: 0, msg: 'success' },
      success({ deleted: 2 }),
      success({ memory, restored: true }),
      success({ events: [memoryAudit], total: 1 }),
    ]);
    expect(recordingCanonicalClient.listMemories).toHaveBeenLastCalledWith({
      space_id: 'store-space-1',
      thread_id: thread.thread_id,
      q: '  launch  ',
      scopes: ['thread'],
      page: 1,
      page_size: 20,
    });
    expect(recordingCanonicalClient.updateMemory).toHaveBeenLastCalledWith({
      space_id: 'store-space-1',
      ...updateRequest,
    });
    expect(recordingCanonicalClient.exportMemories).toHaveBeenCalledWith({
      space_id: 'store-space-1',
      thread_id: thread.thread_id,
      page_size: 100,
    });
  });

  it('preserves Memory AbortError identity at the page boundary', async () => {
    const abortError = new DOMException('Memory request aborted', 'AbortError');
    recordingCanonicalClient.listMemories.mockRejectedValueOnce(abortError);

    const request = taskMemoryService.listTaskThreadMemories({
      thread_id: thread.thread_id,
    });

    await expect(request).rejects.toBe(abortError);
    await expect(request).rejects.toMatchObject({ name: 'AbortError' });
  });

  it('keeps Runtime Doctor, Skill install and resource transports on their owners', async () => {
    expect(taskService.getWorkbenchRuntimeDoctor).toBe(
      generatedOwners.runtimeDoctor,
    );
    expect(workbenchService.getWorkbenchRuntimeDoctor).toBe(
      generatedOwners.runtimeDoctor,
    );
    expect(taskService.installSkillFromArtifact).toBe(
      generatedOwners.installSkillFromArtifact,
    );

    await taskService.getWorkbenchRuntimeDoctor({ space_id: 'space' });
    await taskService.installSkillFromArtifact({
      space_id: 'space',
      thread_id: thread.thread_id,
      artifact_id: artifact.artifact_id,
    });
    const models = await workbenchService.getWorkbenchLLMModels('space');
    const knowledge =
      await workbenchService.listWorkbenchKnowledgeResources('space');
    const databases =
      await workbenchService.listWorkbenchDatabaseResources('space');
    const workflows =
      await workbenchService.listWorkbenchWorkflowResources('space');

    expect(generatedOwners.runtimeDoctor).toHaveBeenCalledOnce();
    expect(generatedOwners.installSkillFromArtifact).toHaveBeenCalledOnce();
    expect(models).toEqual([
      expect.objectContaining({
        model_name: 'model-a',
        workspace_model_id: 'workspace-model-a',
      }),
    ]);
    expect(knowledge).toEqual([{ id: 'kb-1', name: 'Knowledge' }]);
    expect(databases).toEqual([{ id: 'db-1', name: 'Database' }]);
    expect(workflows).toEqual([{ id: 'wf-1', name: 'Workflow' }]);
  });
});
