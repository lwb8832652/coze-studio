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
  createTaskThreadRun,
  sendWorkbenchChat,
  uploadTaskThreadFiles,
  type TaskThreadUploadedFile,
} from './service';

const getThreadFollowUpMetadata = stringifyWorkbenchRunConfig;

export interface CanonicalThreadFollowUpResult {
  kind: 'thread';
  message?: workbenchTask.TaskThreadMessage;
  run?: workbenchTask.TaskThreadRun;
}

interface ThreadFollowUpRunInputOptions {
  payload: WorkbenchComposerSubmitPayload;
  uploadedFiles?: TaskThreadUploadedFile[];
}

const getThreadFollowUpRunInput = ({
  payload,
  uploadedFiles = [],
}: ThreadFollowUpRunInputOptions) =>
  JSON.stringify({
    messages: [{ role: 'user', content: payload.message }],
    uploaded_files: uploadedFiles,
  });

const getThreadFollowUpRunMetadata = (
  payload: WorkbenchComposerSubmitPayload,
) =>
  JSON.stringify({
    source: 'workbench_detail_followup',
    mode: payload.mode,
  });

const createFollowUpIdempotencyKey = (threadId: string) => {
  const requestId =
    globalThis.crypto?.randomUUID?.() ??
    `${Date.now().toString(36)}-${Math.random().toString(36).slice(2)}`;

  return `${threadId}:${requestId}:followup`;
};

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
    const uploadResponse = await uploadTaskThreadFiles({
      thread_id: threadId,
      files: payload.files ?? [],
    });
    const runResponse = await createTaskThreadRun({
      thread_id: threadId,
      input: getThreadFollowUpRunInput({
        payload,
        uploadedFiles: uploadResponse.data?.files ?? [],
      }),
      config: getThreadFollowUpMetadata(payload),
      metadata: getThreadFollowUpRunMetadata(payload),
      message_content: payload.message,
      message_metadata: getThreadFollowUpMetadata(payload),
      idempotency_key: createFollowUpIdempotencyKey(threadId),
    });

    return {
      kind: 'thread',
      message: runResponse.message,
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
