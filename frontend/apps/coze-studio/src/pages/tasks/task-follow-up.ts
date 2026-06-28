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
} from './service';

const getThreadFollowUpMetadata = stringifyWorkbenchRunConfig;

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

const getThreadFollowUpRunInput = (
  payload: WorkbenchComposerSubmitPayload,
  messageId: string,
  historyMessages: Array<{
    role?: string;
    content?: string;
    message_id?: string;
  }> = [],
) => {
  const messages = historyMessages
    .map(normalizeThreadMessageForRunInput)
    .filter((message): message is ThreadRunInputMessage => Boolean(message));

  messages.push(
    {
      role: 'user',
      content: payload.message,
      message_id: messageId,
    },
  );

  return JSON.stringify({
    messages,
  });
};

const listThreadFollowUpHistory = async (threadId: string) => {
  const response = await listTaskThreadMessages({
    thread_id: threadId,
    page: 1,
    page_size: 50,
  });

  return response.data?.messages ?? [];
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
    const appendResponse = await appendTaskThreadMessage({
      thread_id: threadId,
      role: 'user',
      content: payload.message,
      metadata: getThreadFollowUpMetadata(payload),
    });
    const appendedMessageId = appendResponse.data?.message_id || 'pending';

    await createTaskThreadRun({
      thread_id: threadId,
      input: getThreadFollowUpRunInput(
        payload,
        appendedMessageId,
        historyMessages,
      ),
      config: getThreadFollowUpMetadata(payload),
      metadata: getThreadFollowUpRunMetadata(payload, appendedMessageId),
      idempotency_key: `${threadId}:${appendedMessageId}:followup`,
    });

    return;
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
    enable_skills: payload.enable_skills,
    enable_mcp: payload.enable_mcp,
    enable_kbs: payload.enable_kbs,
    enable_databases: payload.enable_databases,
  });
};
