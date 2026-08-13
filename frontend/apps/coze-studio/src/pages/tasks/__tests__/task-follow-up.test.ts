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

import { sendFollowUpMessage } from '../task-follow-up';
import {
  createDefaultWorkbenchResourceSelection,
  createDefaultWorkbenchRuntimeSettings,
} from '../../workbench/components/types';

const mockCreateTaskThreadRun = vi.hoisted(() => vi.fn());
const mockUploadTaskThreadFiles = vi.hoisted(() => vi.fn());

vi.mock('../service', () => ({
  createTaskThreadRun: mockCreateTaskThreadRun,
  uploadTaskThreadFiles: mockUploadTaskThreadFiles,
}));

describe('sendFollowUpMessage', () => {
  beforeEach(() => {
    mockCreateTaskThreadRun.mockReset();
    mockUploadTaskThreadFiles.mockReset();

    mockUploadTaskThreadFiles.mockResolvedValue({
      data: {
        files: [],
        skipped_files: [],
      },
      code: 0,
      msg: '',
    });
    mockCreateTaskThreadRun.mockResolvedValue({
      data: {
        run_id: 'run-follow-up',
        thread_id: 'thread-1',
        status: 'queued',
      },
      message: {
        message_id: 'msg-follow-up',
        thread_id: 'thread-1',
        run_id: 'run-follow-up',
        role: 'user',
        content: '继续分析',
        metadata: '{}',
        created_at: 1717000300000,
      },
      code: 0,
      msg: '',
    });
  });

  it('submits only the current turn and lets the server rebuild thread history', async () => {
    const resourceSelection = {
      ...createDefaultWorkbenchResourceSelection(),
      enable_skills: ['skill-a'],
      enable_mcp: ['tool-a'],
      enable_kbs: ['kb-a'],
      enable_databases: ['database-a'],
    };
    await sendFollowUpMessage({
      payload: {
        message: '继续分析',
        modelType: 100002,
        modelName: 'deepseek-v4-pro',
        runtimeSettings:
          createDefaultWorkbenchRuntimeSettings(resourceSelection),
        ...resourceSelection,
      },
      spaceId: 'space-1',
      threadId: 'thread-1',
      idempotencyKey: 'captured-followup-key',
    });

    const runRequest = mockCreateTaskThreadRun.mock.calls[0]?.[0];
    expect(runRequest).toMatchObject({
      assistant_id: 'agent',
      idempotency_key: 'captured-followup-key',
      submission_v2: {
        schema_version: 'coze.workbench.run_submission.v2',
        kind: 'turn',
        input: { message: '继续分析', uploaded_files: [] },
        composer: {
          model_type: '100002',
          model_name: 'deepseek-v4-pro',
          explicit_enable_skills: ['skill-a'],
          enable_mcp: ['tool-a'],
          enable_kbs: ['kb-a'],
          enable_databases: ['database-a'],
        },
        metadata: { source: 'workbench_detail_followup' },
      },
    });
    [
      'input',
      'config',
      'context',
      'metadata',
      'coze',
      'message_content',
      'message_metadata',
    ].forEach(field => expect(runRequest).not.toHaveProperty(field));
  });

  it('uploads files before creating a thread run', async () => {
    const file = new File(['draft'], 'draft.md', { type: 'text/markdown' });
    mockUploadTaskThreadFiles.mockResolvedValueOnce({
      data: {
        files: [
          {
            file_id: 'file-1',
            file_name: 'draft.md',
            virtual_path: '/uploads/draft.md',
            content_type: 'text/markdown',
            size_bytes: 5,
          },
          {
            file_id: 'file-2',
            file_name: 'notes.md',
            virtual_path: '/uploads/notes.md',
            content_type: 'text/markdown',
            size_bytes: 4,
          },
        ],
        skipped_files: [],
      },
      code: 0,
      msg: '',
    });
    const request = {
      payload: {
        files: [file],
        message: '继续分析附件',
        runtimeSettings: createDefaultWorkbenchRuntimeSettings(),
        ...createDefaultWorkbenchResourceSelection(),
      },
      spaceId: 'space-1',
      threadId: 'thread-1',
    };

    await expect(sendFollowUpMessage(request)).resolves.toMatchObject({
      kind: 'thread',
      run: { run_id: 'run-follow-up' },
    });

    expect(mockUploadTaskThreadFiles).toHaveBeenCalledWith({
      thread_id: 'thread-1',
      space_id: 'space-1',
      files: [file],
    });
    const runRequest = mockCreateTaskThreadRun.mock.calls[0]?.[0];
    expect(runRequest.submission_v2).toMatchObject({
      kind: 'turn',
      input: {
        message: '继续分析附件',
        uploaded_files: [{ file_id: 'file-1' }, { file_id: 'file-2' }],
      },
      metadata: { source: 'workbench_detail_followup' },
    });
    expect(mockCreateTaskThreadRun).toHaveBeenCalledTimes(1);
    expect(mockUploadTaskThreadFiles.mock.invocationCallOrder[0]).toBeLessThan(
      mockCreateTaskThreadRun.mock.invocationCallOrder[0],
    );
  });

  it('does not create a Run when an upload response has no file ID', async () => {
    mockUploadTaskThreadFiles.mockResolvedValueOnce({
      data: {
        files: [{ file_name: 'broken.md', virtual_path: '/broken.md' }],
      },
      code: 0,
      msg: '',
    });

    await expect(
      sendFollowUpMessage({
        payload: {
          files: [new File(['broken'], 'broken.md')],
          message: '继续分析附件',
          runtimeSettings: createDefaultWorkbenchRuntimeSettings(),
          ...createDefaultWorkbenchResourceSelection(),
        },
        spaceId: 'space-1',
        threadId: 'thread-1',
      }),
    ).rejects.toThrow('上传文件缺少 ID');
    expect(mockCreateTaskThreadRun).not.toHaveBeenCalled();
  });
});
