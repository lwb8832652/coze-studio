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

import { type workbenchTask } from '@coze-studio/api-schema';

import { presentTaskThreadTokenUsageResponse } from '../workbench/thread-client/legacy-page-response';
import {
  canonicalThreadClient,
  resolvePageServiceSpaceID,
} from '../workbench/thread-client/canonical-thread-client-singleton';

const createTaskUsageAbortError = () => {
  if (typeof DOMException !== 'undefined') {
    return new DOMException('Task usage request aborted', 'AbortError');
  }

  const error = new Error('Task usage request aborted');
  error.name = 'AbortError';
  return error;
};

export const getTaskThreadTokenUsage = async (
  request: workbenchTask.GetTaskThreadTokenUsageRequest & {
    space_id?: string;
  },
  options?: { signal?: AbortSignal },
): Promise<workbenchTask.GetTaskThreadTokenUsageResponse> => {
  const signal = options?.signal;
  if (signal?.aborted) {
    throw createTaskUsageAbortError();
  }

  return presentTaskThreadTokenUsageResponse(
    await canonicalThreadClient.getTokenUsage({
      ...request,
      ...(signal === undefined ? {} : { signal }),
      space_id: resolvePageServiceSpaceID(request.space_id),
    }),
  ) as unknown as workbenchTask.GetTaskThreadTokenUsageResponse;
};
