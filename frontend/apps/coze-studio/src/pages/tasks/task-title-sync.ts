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

import type { WorkbenchThread } from '../workbench/thread-client';

import { emitWorkspaceTaskThreadUpsert } from './task-thread-events';
import {
  TaskThreadDetailStatus,
  type TaskThreadDetailModel,
} from './task-thread-detail-model';

const mapTaskDetailStatusToThreadStatus = (status: TaskThreadDetailStatus) => {
  switch (status) {
    case TaskThreadDetailStatus.Queued:
      return 'queued';
    case TaskThreadDetailStatus.Running:
      return 'running';
    case TaskThreadDetailStatus.Succeeded:
      return 'succeeded';
    case TaskThreadDetailStatus.Failed:
      return 'failed';
    case TaskThreadDetailStatus.Canceling:
      return 'canceling';
    case TaskThreadDetailStatus.Canceled:
      return 'canceled';
    case TaskThreadDetailStatus.Created:
    default:
      return 'created';
  }
};

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
  if (!spaceID) {
    return;
  }

  const nextTitle = task.title?.trim();
  const titleChanged =
    Boolean(nextTitle) && previousTask.title?.trim() !== nextTitle;
  const statusChanged = previousTask.status !== task.status;
  const thread: Partial<WorkbenchThread> & Pick<WorkbenchThread, 'thread_id'> = {
    thread_id: task.id,
    updated_at: task.updated_at,
  };

  if (titleChanged) {
    thread.title = nextTitle;
  }
  if (statusChanged) {
    thread.status = mapTaskDetailStatusToThreadStatus(task.status);
  }

  emitWorkspaceTaskThreadUpsert({
    mode: 'patch',
    space_id: spaceID,
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
    (nextTask: SetStateAction<TaskThreadDetailModel | undefined>) => {
      const previousTask = taskRef.current;
      const resolvedTask =
        typeof nextTask === 'function' ? nextTask(previousTask) : nextTask;
      taskRef.current = resolvedTask;
      setTask(resolvedTask);

      const nextTitle = resolvedTask?.title?.trim();
      const titleChanged =
        Boolean(nextTitle) && previousTask?.title?.trim() !== nextTitle;
      const statusChanged = previousTask?.status !== resolvedTask?.status;
      if (
        !resolvedTask ||
        previousTask?.id !== resolvedTask.id ||
        (!titleChanged && !statusChanged)
      ) {
        return;
      }

      emitThreadSummaryPatch({
        previousTask,
        spaceID,
        task: nextTitle
          ? { ...resolvedTask, title: nextTitle }
          : resolvedTask,
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
