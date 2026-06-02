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

import { canCancelTask, formatUpdatedTime } from '../index';

describe('TasksPage helpers', () => {
  it('formats backend millisecond timestamps without converting from seconds', () => {
    const timestamp = 1717000300123;

    expect(formatUpdatedTime(timestamp)).toBe(
      new Date(timestamp).toLocaleString(),
    );
  });

  it('allows canceling created tasks', () => {
    expect(canCancelTask(workbenchTask.TaskStatus.Created)).toBe(true);
    expect(canCancelTask(workbenchTask.TaskStatus.Queued)).toBe(true);
    expect(canCancelTask(workbenchTask.TaskStatus.Running)).toBe(true);
  });
});
