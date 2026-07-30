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

import { presentTaskThreadTokenUsageResponse } from '../workbench/thread-client/legacy-page-response';
import { canonicalThreadClient } from '../workbench/thread-client/canonical-thread-client-singleton';
import type {
  GetWorkbenchTokenUsageRequest,
  WorkbenchRunTokenUsageAggregate,
  WorkbenchTokenUsage,
  WorkbenchTokenUsageAggregate,
} from '../workbench/thread-client';

const createTaskUsageAbortError = () => {
  if (typeof DOMException !== 'undefined') {
    return new DOMException('Task usage request aborted', 'AbortError');
  }

  const error = new Error('Task usage request aborted');
  error.name = 'AbortError';
  return error;
};

type GetTaskThreadTokenUsageRequest = Omit<
  GetWorkbenchTokenUsageRequest,
  'signal'
>;

export interface GetTaskThreadTokenUsageResponse {
  data?: {
    usage: WorkbenchTokenUsage[];
    total: number;
    aggregate: WorkbenchTokenUsageAggregate;
    run_aggregates: WorkbenchRunTokenUsageAggregate[];
  };
  code: number;
  msg: string;
}

export const getTaskThreadTokenUsage = async (
  request: GetTaskThreadTokenUsageRequest,
  options?: { signal?: AbortSignal },
): Promise<GetTaskThreadTokenUsageResponse> => {
  const signal = options?.signal;
  if (signal?.aborted) {
    throw createTaskUsageAbortError();
  }
  const spaceID = String(request.space_id ?? '').trim();
  if (!spaceID) {
    throw new Error('Task usage workspace scope is required');
  }

  return presentTaskThreadTokenUsageResponse(
    await canonicalThreadClient.getTokenUsage({
      ...request,
      ...(signal === undefined ? {} : { signal }),
      space_id: spaceID,
    }),
  );
};
