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
import { createDefaultWorkbenchRuntimeSettings } from '../../workbench/components/types';

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
    await sendFollowUpMessage({
      payload: {
        message: '继续分析',
        mode: 'pro',
        runtimeSettings: createDefaultWorkbenchRuntimeSettings(),
      },
      threadId: 'thread-1',
    });

    const runRequest = mockCreateTaskThreadRun.mock.calls[0]?.[0];
    expect(JSON.parse(runRequest.input)).toMatchObject({
      messages: [
        {
          role: 'user',
          content: '继续分析',
        },
      ],
    });
    expect(runRequest).toMatchObject({
      message_content: '继续分析',
      message_metadata: expect.any(String),
      idempotency_key: expect.stringMatching(/^thread-1:.+:followup$/),
    });
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
        mode: 'pro' as const,
        runtimeSettings: createDefaultWorkbenchRuntimeSettings(),
      },
      threadId: 'thread-1',
    };

    await expect(sendFollowUpMessage(request)).resolves.toMatchObject({
      kind: 'thread',
      run: { run_id: 'run-follow-up' },
    });

    expect(mockUploadTaskThreadFiles).toHaveBeenCalledWith({
      thread_id: 'thread-1',
      files: [file],
    });
    const runRequest = mockCreateTaskThreadRun.mock.calls[0]?.[0];
    expect(JSON.parse(runRequest.input)).toMatchObject({
      messages: [{ role: 'user', content: '继续分析附件' }],
      uploaded_files: [
        {
          file_id: 'file-1',
          file_name: 'draft.md',
          virtual_path: '/uploads/draft.md',
        },
      ],
    });
    expect(mockCreateTaskThreadRun).toHaveBeenCalledTimes(1);
  });
});
