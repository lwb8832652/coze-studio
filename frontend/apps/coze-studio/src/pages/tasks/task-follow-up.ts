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

import type {
  WorkbenchMessage,
  WorkbenchRun,
} from '../workbench/thread-client';
import {
  createTurnSubmissionV2,
  requireUploadedFileIDs,
  type WorkbenchComposerSubmitPayload,
} from '../workbench/components/types';
import { createTaskThreadRun, uploadTaskThreadFiles } from './service';

export interface CanonicalThreadFollowUpResult {
  kind: 'thread';
  message?: WorkbenchMessage;
  run?: WorkbenchRun;
}

export const createFollowUpIdempotencyKey = (threadId: string) => {
  const requestId =
    globalThis.crypto?.randomUUID?.() ??
    `${Date.now().toString(36)}-${Math.random().toString(36).slice(2)}`;

  return `${threadId}:${requestId}:followup`;
};

export const sendFollowUpMessage = async ({
  payload,
  spaceId,
  threadId,
  idempotencyKey,
}: {
  payload: WorkbenchComposerSubmitPayload;
  spaceId: string;
  threadId: string;
  idempotencyKey?: string;
}) => {
  const uploadResponse = await uploadTaskThreadFiles({
    thread_id: threadId,
    space_id: spaceId,
    files: payload.files ?? [],
  });
  const runResponse = await createTaskThreadRun({
    thread_id: threadId,
    space_id: spaceId,
    assistant_id: 'agent',
    submission_v2: createTurnSubmissionV2(
      payload,
      requireUploadedFileIDs(uploadResponse.data?.files ?? []),
      'workbench_detail_followup',
    ),
    idempotency_key: idempotencyKey ?? createFollowUpIdempotencyKey(threadId),
  });

  return {
    kind: 'thread',
    message: runResponse.message,
    run: runResponse.data,
  } satisfies CanonicalThreadFollowUpResult;
};
