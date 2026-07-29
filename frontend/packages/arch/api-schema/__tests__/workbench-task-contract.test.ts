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
  it('declares the complete scheduled task center contract', () => {
    // eslint-disable-next-line security/detect-non-literal-fs-filename -- Static fixture path.
    const source = readFileSync(taskThriftPath, 'utf8');
    const requiredMethods = [
      'CreateScheduledTask',
      'UpdateScheduledTask',
      'GetScheduledTask',
      'ListScheduledTasks',
      'DeleteScheduledTask',
      'EnableScheduledTask',
      'DisableScheduledTask',
      'ExecuteScheduledTask',
      'ListScheduledTaskExecutions',
      'ListScheduledTaskTargets',
      'ListScheduledTaskCronPresets',
    ];

    for (const method of requiredMethods) {
      expect(source).toContain(`${method}(`);
    }
  });

  it('maps scheduled task list filters and task actions', () => {
    expect(workbenchTask.ListScheduledTasks.meta).toMatchObject({
      method: 'GET',
      reqMapping: {
        query: expect.arrayContaining([
          'space_id',
          'target_type',
          'keyword',
          'status',
          'page',
          'page_size',
        ]),
      },
      url: '/api/workbench/scheduled_tasks',
    });
    expect(workbenchTask.ExecuteScheduledTask.meta).toMatchObject({
      method: 'POST',
      url: '/api/workbench/scheduled_tasks/:task_id/execute',
    });
  });
});
