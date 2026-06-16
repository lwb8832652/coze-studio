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
  type WorkbenchComposerSubmitPayload,
} from '../workbench/components/types';
import {
  appendTaskThreadMessage,
  createTaskThreadRun,
  sendWorkbenchChat,
} from './service';

const getThreadFollowUpMetadata = (payload: WorkbenchComposerSubmitPayload) =>
  JSON.stringify({
    mode: payload.mode,
    model_type: payload.modelType,
    model_name: payload.modelName,
    enable_skills: payload.enable_skills,
    enable_mcp: payload.enable_mcp,
    enable_kbs: payload.enable_kbs,
    enable_databases: payload.enable_databases,
  });

const getThreadFollowUpRunInput = (
  payload: WorkbenchComposerSubmitPayload,
  messageId: string,
) =>
  JSON.stringify({
    messages: [
      {
        role: 'user',
        content: payload.message,
        message_id: messageId,
      },
    ],
  });

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
    const appendResponse = await appendTaskThreadMessage({
      thread_id: threadId,
      role: 'user',
      content: payload.message,
      metadata: getThreadFollowUpMetadata(payload),
    });
    const appendedMessageId = appendResponse.data?.message_id || 'pending';

    await createTaskThreadRun({
      thread_id: threadId,
      input: getThreadFollowUpRunInput(payload, appendedMessageId),
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
    enable_skills: payload.enable_skills,
    enable_mcp: payload.enable_mcp,
    enable_kbs: payload.enable_kbs,
    enable_databases: payload.enable_databases,
  });
};
