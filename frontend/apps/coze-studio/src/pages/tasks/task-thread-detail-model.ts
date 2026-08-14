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

import type { WorkbenchAdaptiveExecution } from '../workbench/thread-client';

export enum TaskThreadDetailStatus {
  Created = 1,
  Queued = 2,
  Running = 3,
  Succeeded = 4,
  Failed = 5,
  Canceling = 6,
  Canceled = 7,
}

export interface TaskThreadDetailEvent {
  id: string;
  thread_id: string;
  event_type: string;
  payload?: string;
  created_at: number;
  run_id?: string;
}

export interface TaskThreadDetailModel {
  id: string;
  space_id: string;
  creator_id?: string;
  can_edit?: boolean;
  conversation_id?: string;
  title: string;
  status: TaskThreadDetailStatus;
  progress: number;
  input?: string;
  result?: string;
  error?: string;
  created_at: number;
  updated_at: number;
  adaptive_execution?: WorkbenchAdaptiveExecution;
}

export const isTaskThreadDetailReadOnly = ({
  task,
  userID,
}: {
  task?: TaskThreadDetailModel;
  userID?: string;
}): boolean => {
  if (typeof task?.can_edit === 'boolean') {
    return !task.can_edit;
  }

  return Boolean(task?.creator_id && userID && task.creator_id !== userID);
};
