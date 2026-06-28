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

import { readFileSync } from 'node:fs';

import { describe, expect, it } from 'vitest';

import { workbenchTask } from '../src';

const taskThriftPath = new URL(
  '../../../../../idl/workbench/task.thrift',
  import.meta.url,
);

describe('workbench task api contract source', () => {
  it('declares task-thread run clients in thrift before generated clients use them', () => {
    // eslint-disable-next-line security/detect-non-literal-fs-filename -- Static fixture path.
    const source = readFileSync(taskThriftPath, 'utf8');
    const requiredMethods = [
      'ListTaskThreads',
      'GetTaskThread',
      'ListTaskThreadMessages',
      'AppendTaskThreadMessage',
      'ListTaskThreadRuns',
      'ListTaskThreadRunEvents',
      'GetTaskThreadTokenUsage',
      'ListTaskThreadArtifacts',
      'GetTaskThreadArtifactSignedURL',
      'CreateTaskThreadRun',
      'ResumeTaskThreadRun',
      'CancelTaskThreadRun',
      'RetryTaskThreadSubagentRun',
    ];

    for (const method of requiredMethods) {
      expect(source).toContain(`${method}(`);
    }
  });

  it('maps parent_run_id as a query parameter for child run listing', () => {
    expect(workbenchTask.ListTaskThreadRuns.meta).toMatchObject({
      method: 'GET',
      reqMapping: {
        path: ['thread_id'],
        query: ['parent_run_id', 'status', 'page', 'page_size'],
      },
      url: '/api/workbench/task_threads/:thread_id/runs',
    });
  });
});
