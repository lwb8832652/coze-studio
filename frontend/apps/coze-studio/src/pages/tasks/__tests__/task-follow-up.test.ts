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

const mockAppendTaskThreadMessage = vi.hoisted(() => vi.fn());
const mockCreateTaskThreadRun = vi.hoisted(() => vi.fn());
const mockListTaskThreadMessages = vi.hoisted(() => vi.fn());
const mockSendWorkbenchChat = vi.hoisted(() => vi.fn());
const mockUploadTaskThreadFiles = vi.hoisted(() => vi.fn());

vi.mock('../service', () => ({
  appendTaskThreadMessage: mockAppendTaskThreadMessage,
  createTaskThreadRun: mockCreateTaskThreadRun,
  listTaskThreadMessages: mockListTaskThreadMessages,
  sendWorkbenchChat: mockSendWorkbenchChat,
  uploadTaskThreadFiles: mockUploadTaskThreadFiles,
}));

describe('sendFollowUpMessage', () => {
  beforeEach(() => {
    mockAppendTaskThreadMessage.mockReset();
    mockCreateTaskThreadRun.mockReset();
    mockListTaskThreadMessages.mockReset();
    mockSendWorkbenchChat.mockReset();
    mockUploadTaskThreadFiles.mockReset();

    mockUploadTaskThreadFiles.mockResolvedValue({
      data: {
        files: [],
        skipped_files: [],
      },
      code: 0,
      msg: '',
    });
    mockAppendTaskThreadMessage.mockResolvedValue({
      data: {
        message_id: 'msg-follow-up',
        thread_id: 'thread-1',
        run_id: '',
        role: 'user',
        content: '继续分析',
        metadata: '{}',
        created_at: 1717000300000,
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
      code: 0,
      msg: '',
    });
  });

  it('loads every page of canonical thread history before creating a follow-up run', async () => {
    mockListTaskThreadMessages
      .mockResolvedValueOnce({
        data: {
          messages: [
            {
              message_id: 'msg-1',
              role: 'user',
              content: '第一轮问题',
            },
            {
              message_id: 'msg-2',
              role: 'assistant',
              content: '第一轮回答',
            },
          ],
          total: 3,
        },
        code: 0,
        msg: '',
      })
      .mockResolvedValueOnce({
        data: {
          messages: [
            {
              message_id: 'msg-3',
              role: 'assistant',
              content: '第二轮补充',
            },
          ],
          total: 3,
        },
        code: 0,
        msg: '',
      });

    await sendFollowUpMessage({
      activeTaskId: 'thread-1',
      isCanonicalThreadDetail: true,
      payload: {
        message: '继续分析',
        mode: 'pro',
        runtimeSettings: createDefaultWorkbenchRuntimeSettings(),
      },
      spaceId: 'space-1',
      threadId: 'thread-1',
    });

    expect(mockListTaskThreadMessages).toHaveBeenNthCalledWith(1, {
      thread_id: 'thread-1',
      page: 1,
      page_size: 200,
    });
    expect(mockListTaskThreadMessages).toHaveBeenNthCalledWith(2, {
      thread_id: 'thread-1',
      page: 2,
      page_size: 200,
    });

    const runRequest = mockCreateTaskThreadRun.mock.calls[0]?.[0];
    expect(JSON.parse(runRequest.input)).toMatchObject({
      messages: [
        {
          role: 'user',
          content: '第一轮问题',
          message_id: 'msg-1',
        },
        {
          role: 'assistant',
          content: '第一轮回答',
          message_id: 'msg-2',
        },
        {
          role: 'assistant',
          content: '第二轮补充',
          message_id: 'msg-3',
        },
        {
          role: 'user',
          content: '继续分析',
          message_id: 'msg-follow-up',
        },
      ],
    });
  });
});
