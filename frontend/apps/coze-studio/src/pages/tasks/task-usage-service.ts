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

import { workbenchTask } from '@coze-studio/api-schema';

const createTaskUsageAbortError = () => {
  if (typeof DOMException !== 'undefined') {
    return new DOMException('Task usage request aborted', 'AbortError');
  }

  const error = new Error('Task usage request aborted');
  error.name = 'AbortError';
  return error;
};

export const getTaskThreadTokenUsage = (
  request: workbenchTask.GetTaskThreadTokenUsageRequest,
  options?: { signal?: AbortSignal },
): Promise<workbenchTask.GetTaskThreadTokenUsageResponse> => {
  const api = workbenchTask.GetTaskThreadTokenUsage.withAbort();
  const signal = options?.signal;

  if (!signal) {
    return api(request);
  }
  if (signal.aborted) {
    return Promise.reject(createTaskUsageAbortError());
  }

  return new Promise((resolve, reject) => {
    let settled = false;
    const finish = (
      callback: (
        value: workbenchTask.GetTaskThreadTokenUsageResponse | unknown,
      ) => void,
      value: workbenchTask.GetTaskThreadTokenUsageResponse | unknown,
    ) => {
      if (settled) {
        return;
      }
      settled = true;
      signal.removeEventListener('abort', handleAbort);
      callback(value);
    };
    const handleAbort = () => {
      api.abort();
      finish(reject, createTaskUsageAbortError());
    };

    signal.addEventListener('abort', handleAbort, { once: true });
    void api(request).then(
      response => finish(resolve, response),
      error => finish(reject, error),
    );
  });
};
