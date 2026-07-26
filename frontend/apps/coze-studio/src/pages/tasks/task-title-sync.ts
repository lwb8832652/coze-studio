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

import { useCallback, useRef, type Dispatch, type SetStateAction } from 'react';

import type { workbenchTask } from '@coze-studio/api-schema';

import { emitWorkspaceTaskThreadUpsert } from './task-thread-events';
import type { TaskThreadDetailModel } from './task-thread-detail-model';

const buildThreadTitleUpdatedTask = ({
  currentTask,
  threadId,
  title,
}: {
  currentTask?: TaskThreadDetailModel;
  threadId: string;
  title: string;
}) => {
  const normalizedTitle = title.trim();
  if (!currentTask || currentTask.id !== threadId || !normalizedTitle) {
    return undefined;
  }

  return {
    ...currentTask,
    title: normalizedTitle,
    updated_at: Date.now(),
  };
};

const emitThreadSummaryPatch = ({
  previousTask,
  spaceID,
  task,
}: {
  previousTask: TaskThreadDetailModel;
  spaceID?: string;
  task: TaskThreadDetailModel;
}) => {
  const nextTitle = task.title?.trim();
  const titleChanged =
    Boolean(nextTitle) && previousTask.title?.trim() !== nextTitle;
  const statusChanged = previousTask.status !== task.status;
  const thread: Partial<workbenchTask.TaskThread> &
    Pick<workbenchTask.TaskThread, 'thread_id'> = {
    thread_id: task.id,
    updated_at: task.updated_at,
  };

  if (titleChanged) {
    thread.title = nextTitle;
  }
  if (statusChanged) {
    thread.status = task.status;
  }

  emitWorkspaceTaskThreadUpsert({
    mode: 'patch',
    space_id: task.space_id || spaceID || '',
    thread,
  });
};

export const useTaskThreadTitleSync = ({
  setTask,
  spaceID,
}: {
  setTask: Dispatch<SetStateAction<TaskThreadDetailModel | undefined>>;
  spaceID?: string;
}) => {
  const taskRef = useRef<TaskThreadDetailModel | undefined>();
  const setCurrentTask = useCallback(
    (nextTask?: TaskThreadDetailModel) => {
      const previousTask = taskRef.current;
      taskRef.current = nextTask;
      setTask(nextTask);

      const nextTitle = nextTask?.title?.trim();
      const titleChanged =
        Boolean(nextTitle) && previousTask?.title?.trim() !== nextTitle;
      const statusChanged = previousTask?.status !== nextTask?.status;
      if (
        !nextTask ||
        previousTask?.id !== nextTask.id ||
        (!titleChanged && !statusChanged)
      ) {
        return;
      }

      emitThreadSummaryPatch({
        previousTask,
        spaceID,
        task: nextTitle ? { ...nextTask, title: nextTitle } : nextTask,
      });
    },
    [setTask, spaceID],
  );
  const handleThreadTitleUpdated = useCallback(
    ({ threadId, title }: { threadId: string; title: string }) => {
      const nextTask = buildThreadTitleUpdatedTask({
        currentTask: taskRef.current,
        threadId,
        title,
      });
      if (!nextTask) {
        return;
      }

      setCurrentTask(nextTask);
    },
    [setCurrentTask],
  );

  return {
    handleThreadTitleUpdated,
    setCurrentTask,
  };
};
