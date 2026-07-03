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

import type { workbenchTask } from '@coze-studio/api-schema';

import {
  mapModeToChatMode,
  stringifyWorkbenchRunConfig,
  type WorkbenchComposerSubmitPayload,
} from '../workbench/components/types';
import {
  appendTaskThreadMessage,
  createTaskThreadRun,
  listTaskThreadMessages,
  sendWorkbenchChat,
  uploadTaskThreadFiles,
  type TaskThreadUploadedFile,
} from './service';

const getThreadFollowUpMetadata = stringifyWorkbenchRunConfig;
const THREAD_FOLLOW_UP_HISTORY_PAGE_SIZE = 200;

export interface CanonicalThreadFollowUpResult {
  kind: 'thread';
  message?: workbenchTask.TaskThreadMessage;
  run?: workbenchTask.TaskThreadRun;
}

interface ThreadRunInputMessage {
  role: 'user' | 'assistant';
  content: string;
  message_id?: string;
}

const normalizeThreadMessageForRunInput = (message: {
  role?: string;
  content?: string;
  message_id?: string;
}): ThreadRunInputMessage | undefined => {
  const role = String(message.role ?? '')
    .trim()
    .toLowerCase();
  if (role !== 'user' && role !== 'assistant') {
    return undefined;
  }

  const content = String(message.content ?? '');
  if (!content.trim()) {
    return undefined;
  }

  return {
    role,
    content,
    ...(message.message_id ? { message_id: message.message_id } : {}),
  };
};

const appendThreadRunInputMessage = (
  messages: ThreadRunInputMessage[],
  message: ThreadRunInputMessage,
) => {
  const previous = messages[messages.length - 1];

  if (message.role === 'user' && previous?.role === 'user') {
    messages[messages.length - 1] = message;
    return;
  }

  messages.push(message);
};

interface ThreadFollowUpRunInputOptions {
  payload: WorkbenchComposerSubmitPayload;
  messageId: string;
  historyMessages?: Array<{
    role?: string;
    content?: string;
    message_id?: string;
  }>;
  uploadedFiles?: TaskThreadUploadedFile[];
}

const getThreadFollowUpRunInput = ({
  payload,
  messageId,
  historyMessages = [],
  uploadedFiles = [],
}: ThreadFollowUpRunInputOptions) => {
  const messages: ThreadRunInputMessage[] = [];

  historyMessages.forEach(message => {
    const normalizedMessage = normalizeThreadMessageForRunInput(message);

    if (!normalizedMessage) {
      return;
    }

    appendThreadRunInputMessage(messages, normalizedMessage);
  });

  appendThreadRunInputMessage(messages, {
    role: 'user',
    content: payload.message,
    message_id: messageId,
  });

  return JSON.stringify({
    messages,
    uploaded_files: uploadedFiles,
  });
};

const listThreadFollowUpHistory = async (threadId: string) => {
  const messages: workbenchTask.TaskThreadMessage[] = [];
  let page = 1;
  let total = 0;

  do {
    const response = await listTaskThreadMessages({
      thread_id: threadId,
      page,
      page_size: THREAD_FOLLOW_UP_HISTORY_PAGE_SIZE,
    });
    const pageMessages = response.data?.messages ?? [];

    messages.push(...pageMessages);
    total = response.data?.total ?? messages.length;
    if (!pageMessages.length) {
      break;
    }
    page += 1;
  } while (messages.length < total);

  return messages;
};

const getThreadFollowUpRunMetadata = (
  payload: WorkbenchComposerSubmitPayload,
  messageId: string,
) =>
  JSON.stringify({
    source: 'workbench_detail_followup',
    appended_message_id: messageId,
    mode: payload.mode,
  });

export const sendFollowUpMessage = async ({
  activeTaskId,
  isCanonicalThreadDetail,
  payload,
  spaceId,
  threadId,
}: {
  activeTaskId: string;
  isCanonicalThreadDetail: boolean;
  payload: WorkbenchComposerSubmitPayload;
  spaceId: string;
  threadId: string;
}) => {
  if (isCanonicalThreadDetail) {
    const historyMessages = await listThreadFollowUpHistory(threadId);
    const uploadResponse = await uploadTaskThreadFiles({
      thread_id: threadId,
      files: payload.files ?? [],
    });
    const appendResponse = await appendTaskThreadMessage({
      thread_id: threadId,
      role: 'user',
      content: payload.message,
      metadata: getThreadFollowUpMetadata(payload),
    });
    const appendedMessageId = appendResponse.data?.message_id || 'pending';

    const runResponse = await createTaskThreadRun({
      thread_id: threadId,
      input: getThreadFollowUpRunInput({
        payload,
        messageId: appendedMessageId,
        historyMessages,
        uploadedFiles: uploadResponse.data?.files ?? [],
      }),
      config: getThreadFollowUpMetadata(payload),
      metadata: getThreadFollowUpRunMetadata(payload, appendedMessageId),
      idempotency_key: `${threadId}:${appendedMessageId}:followup`,
    });

    return {
      kind: 'thread',
      message: appendResponse.data,
      run: runResponse.data,
    } satisfies CanonicalThreadFollowUpResult;
  }

  if (payload.files?.length) {
    throw new Error('当前任务详情暂不支持附件追问');
  }

  await sendWorkbenchChat({
    space_id: spaceId,
    task_id: activeTaskId,
    message: payload.message,
    mode: mapModeToChatMode(payload.mode),
    ...(payload.modelType
      ? {
          model_type: String(payload.modelType),
          model_name: payload.modelName,
        }
      : {}),
    runtime_settings: stringifyWorkbenchRunConfig(payload),
    ...(payload.enable_skills
      ? {
          enable_skills: payload.enable_skills,
        }
      : {}),
    enable_mcp: payload.enable_mcp,
    enable_kbs: payload.enable_kbs,
    enable_databases: payload.enable_databases,
  });

  return undefined;
};
